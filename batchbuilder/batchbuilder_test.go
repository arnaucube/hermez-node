package batchbuilder

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hermeznetwork/hermez-node/db/statedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchBuilder(t *testing.T) {
	dir, err := ioutil.TempDir("", "tmpdb")
	require.Nil(t, err)
	defer assert.Nil(t, os.RemoveAll(dir))

	synchDB, err := statedb.NewStateDB(statedb.Config{Path: dir, Keep: 128,
		Type: statedb.TypeBatchBuilder, NLevels: 0})
	assert.Nil(t, err)

	bbDir, err := ioutil.TempDir("", "tmpBatchBuilderDB")
	require.Nil(t, err)
	defer assert.Nil(t, os.RemoveAll(bbDir))
	_, err = NewBatchBuilder(bbDir, synchDB, 0, 32)
	assert.Nil(t, err)
}
