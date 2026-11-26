//go:build integration

package repl //nolint:testpackage // Integration tests exercise unexported replication internals.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/percona/percona-clustersync-mongodb/mdb"
	"github.com/percona/percona-clustersync-mongodb/pcsm/catalog"
)

// The server accepts a top-level field name that starts with "$" on insert, but neither in
// a replacement nor in $set. Both bulk writers must still apply such events.
//
//nolint:paralleltest // MongoDB testcontainers are serialized to avoid Docker resource contention.
func TestBulkWrite_AppliesDocumentsWithDollarPrefixedField(t *testing.T) {
	runReplIntegrationTest(t, func(ctx context.Context) {
		// Given
		client, err := mongo.Connect(options.Client().
			ApplyURI(newReplTestReplicaSet(ctx, t)).
			SetServerSelectionTimeout(30 * time.Second))
		require.NoError(t, err)
		t.Cleanup(func() {
			disconnectReplTestClient(t, client)
		})

		version, err := mdb.Version(ctx, client)
		require.NoError(t, err)

		opts := &Options{}
		opts.applyDefaults()

		for _, collectionBulk := range []bool{true, false} {
			if !collectionBulk && !mdb.Support(version).ClientBulkWrite() {
				continue
			}

			// Source and target are two collections of the same deployment.
			ns := catalog.Namespace{Database: "dollar_field_test", Collection: "source"}
			if collectionBulk {
				ns.Database += "_collection_bulk"
			}

			source := client.Database(ns.Database).Collection(ns.Collection)
			target := client.Database(ns.Database).Collection("target")
			key := bson.D{{"_id", 1}}

			targetDoc := func() bson.D {
				var doc bson.D
				require.NoError(t, target.FindOne(ctx, key).Decode(&doc))

				return doc
			}

			_, err = target.InsertOne(ctx, bson.D{{"_id", 1}, {"a", 1}})
			require.NoError(t, err)

			w := newWorker(0, opts, client, client, collectionBulk, false, make(chan error, 1))
			targetNS := catalog.Namespace{Database: ns.Database, Collection: "target"}

			// When a replace event carries such a field
			replacement := bson.D{{"_id", int32(1)}, {"a", int32(2)}, {"$price", int32(3)}}
			raw, err := bson.Marshal(replacement)
			require.NoError(t, err)

			documentKey, err := bson.Marshal(key)
			require.NoError(t, err)

			bw := w.newBulkWriter()
			bw.Replace(targetNS, &ReplaceEvent{DocumentKey: documentKey, FullDocument: raw})
			_, err = bw.Do(ctx, client)

			// Then it is applied
			require.NoError(t, err)
			assert.Equal(t, replacement, targetDoc())

			// When an insert event for it is replayed over the existing document
			bw = w.newBulkWriter()
			bw.Insert(targetNS, &InsertEvent{DocumentKey: key, FullDocument: raw})
			_, err = bw.Do(ctx, client)

			// Then it is applied as well
			require.NoError(t, err)
			assert.Equal(t, replacement, targetDoc())

			// When an update sets such a field, the document is refetched from source
			sourceDoc := bson.D{{"_id", int32(1)}, {"a", int32(2)}, {"$price", int32(4)}}
			_, err = source.InsertOne(ctx, sourceDoc)
			require.NoError(t, err)

			require.NoError(t, handleRecoverableUpdateError(ctx, client, target, ns, key, false))
			assert.Equal(t, sourceDoc, targetDoc())
		}
	})
}
