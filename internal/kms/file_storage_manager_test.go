package kms

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/iden3/go-iden3-core/v2/w3c"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveKeyMaterialToFile_Success(t *testing.T) {
	tmpFile, err := createTestFile(t)
	assert.NoError(t, err)
	//nolint:errcheck
	defer os.Remove(tmpFile.Name())

	ls := NewFileStorageManager(tmpFile.Name())
	ctx := context.Background()
	keyMaterial := map[string]string{jsonKeyType: string(KeyTypeEthereum), jsonKeyData: "0xABC123"}
	id := "key1"

	err = ls.SaveKeyMaterial(ctx, keyMaterial, id)
	require.NoError(t, err)

	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)

	var fileContent []localStorageProviderFileContent
	err = json.Unmarshal(content, &fileContent)
	require.NoError(t, err)

	assert.Equal(t, 1, len(fileContent))
	assert.Equal(t, id, fileContent[0].KeyPath)
	assert.Equal(t, ethereum, fileContent[0].KeyType)
	assert.Equal(t, keyMaterial[jsonKeyData], fileContent[0].PrivateKey)
}

func TestSaveKeyMaterialToFile_FailOnFileWrite(t *testing.T) {
	ls := NewFileStorageManager("/path/to/non/existent/file")
	ctx := context.Background()
	keyMaterial := map[string]string{"type": "ethereum", "data": "0xABC123"}
	id := "key1"
	err := ls.SaveKeyMaterial(ctx, keyMaterial, id)
	assert.Error(t, err)
}

func TestSearchByIdentityInFile_ReturnsKeyIDsOnMatch(t *testing.T) {
	tmpFile, err := createTestFile(t)
	assert.NoError(t, err)
	//nolint:errcheck
	defer os.Remove(tmpFile.Name())

	identity := "did:polygonid:polygon:amoy:2qQ68JkRcf3ybQNvgRV9BP6qLgBrXmUezqBi4wsEuV"
	fileContent := []localStorageProviderFileContent{
		{KeyPath: identity + "/ETH:0347fe70a2a9b752e8012d72851c35a13a1423bcdac4bde6ec036e1ea9317b36ac", KeyType: string(ethereum), PrivateKey: "0xABC123"},
		{KeyPath: "keys/" + identity + "/BJJ:cecf34ed27074e121f1e8a8cc75954ab2b28506258b87b3c9a20e33461f4b12a", KeyType: string(babyjubjub), PrivateKey: "0xDEF456"},
	}

	content, err := json.Marshal(fileContent)
	require.NoError(t, err)
	//nolint:all
	err = os.WriteFile("./kms.json", content, 0644)
	require.NoError(t, err)

	ls := NewFileStorageManager(tmpFile.Name())
	ctx := context.Background()
	did, err := w3c.ParseDID(identity)
	require.NoError(t, err)

	keyIDs, err := ls.searchByIdentity(ctx, *did, KeyTypeEthereum)
	require.NoError(t, err)
	require.Len(t, keyIDs, 1)
	assert.Equal(t, KeyID{Type: KeyTypeEthereum, ID: identity + "/ETH:0347fe70a2a9b752e8012d72851c35a13a1423bcdac4bde6ec036e1ea9317b36ac"}, keyIDs[0])

	keyIDs, err = ls.searchByIdentity(ctx, *did, KeyTypeBabyJubJub)
	require.NoError(t, err)
	require.Len(t, keyIDs, 1)
	assert.Equal(t, KeyID{Type: KeyTypeBabyJubJub, ID: "keys/" + identity + "/BJJ:cecf34ed27074e121f1e8a8cc75954ab2b28506258b87b3c9a20e33461f4b12a"}, keyIDs[0])
}

//nolint:lll
func TestSearchByIdentityInFile_ReturnsErrorOnFileReadFailure(t *testing.T) {
	ls := NewFileStorageManager("/path/to/nonexistent/file")
	ctx := context.Background()
	did, err := w3c.ParseDID("did:polygonid:polygon:amoy:2qQ68JkRcf3ybQNvgRV9BP6qLgBrXmUezqBi4wsEuV")
	require.NoError(t, err)
	_, err = ls.searchByIdentity(ctx, *did, KeyTypeEthereum)
	assert.Error(t, err)
}

func TestSearchByIdentityInFile_ReturnsEmptySliceWhenNoMatch(t *testing.T) {
	tmpFile, err := createTestFile(t)
	assert.NoError(t, err)
	//nolint:errcheck
	defer os.Remove(tmpFile.Name())

	fileContent := []localStorageProviderFileContent{
		{KeyPath: "key/did:example:456", KeyType: string(KeyTypeEthereum), PrivateKey: "0xABC123"},
	}
	content, err := json.Marshal(fileContent)
	require.NoError(t, err)
	//nolint:all
	err = os.WriteFile("./kms.json", content, 0644)
	require.NoError(t, err)

	ls := NewFileStorageManager(tmpFile.Name())
	ctx := context.Background()

	did, err := w3c.ParseDID("did:polygonid:polygon:amoy:2qQ68JkRcf3ybQNvgRV9BP6qLgBrXmUezqBi4wsEuV")
	require.NoError(t, err)

	keyIDs, err := ls.searchByIdentity(ctx, *did, KeyTypeEthereum)
	require.NoError(t, err)
	assert.Empty(t, keyIDs)
}

//nolint:lll
func TestSearchPrivateKeyInFile_ReturnsPrivateKeyOnMatch(t *testing.T) {
	tmpFile, err := createTestFile(t)
	assert.NoError(t, err)
	//nolint:errcheck
	defer os.Remove(tmpFile.Name())

	fileContent := []localStorageProviderFileContent{
		{KeyPath: "key1", KeyType: "ETH", PrivateKey: "0xABC123"},
	}
	content, err := json.Marshal(fileContent)
	require.NoError(t, err)
	//nolint:all
	err = os.WriteFile("./kms.json", content, 0644)
	require.NoError(t, err)

	ls := NewFileStorageManager(tmpFile.Name())
	ctx := context.Background()

	privateKey, err := ls.searchPrivateKey(ctx, KeyID{ID: "key1"})
	require.NoError(t, err)
	assert.Equal(t, "0xABC123", privateKey)
}

//nolint:lll
func TestSearchPrivateKeyInFile_ReturnsErrorWhenKeyNotFound(t *testing.T) {
	tmpFile, err := createTestFile(t)
	assert.NoError(t, err)
	//nolint:errcheck
	defer os.Remove(tmpFile.Name())

	fileContent := []localStorageProviderFileContent{
		{KeyPath: "key1", KeyType: "Ethereum", PrivateKey: "0xABC123"},
	}
	content, err := json.Marshal(fileContent)
	require.NoError(t, err)
	//nolint:all
	err = os.WriteFile("./kms.json", content, 0644)
	require.NoError(t, err)

	ls := NewFileStorageManager(tmpFile.Name())
	ctx := context.Background()

	_, err = ls.searchPrivateKey(ctx, KeyID{ID: "key2"})
	assert.Error(t, err)
}

func Test_GetKeyMaterial(t *testing.T) {
	tmpFile, err := createTestFile(t)
	assert.NoError(t, err)
	//nolint:errcheck
	defer os.Remove(tmpFile.Name())

	ls := NewFileStorageManager(tmpFile.Name())
	ctx := context.Background()

	t.Run("should return key material", func(t *testing.T) {
		did := randomDID(t)
		privateKey := "9d7abdd5a43573ab9b623c50b9fc8f4357329d3009fe0fc22c8931161d98a03d"
		id := getKeyID(&did, KeyTypeBabyJubJub, "BJJ:2290140c920a31a596937095f18a9ae15c1fe7091091be485f353968a4310380")

		err = ls.SaveKeyMaterial(ctx, map[string]string{
			jsonKeyType: string(KeyTypeBabyJubJub),
			jsonKeyData: privateKey,
		}, id)
		assert.NoError(t, err)

		keyMaterial, err := ls.getKeyMaterial(ctx, KeyID{
			Type: babyjubjub,
			ID:   id,
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			jsonKeyType: string(babyjubjub),
			jsonKeyData: privateKey,
		}, keyMaterial)
	})

	t.Run("should return an error", func(t *testing.T) {
		_, err := ls.getKeyMaterial(ctx, KeyID{
			Type: babyjubjub,
			ID:   "wrong_id",
		})
		require.Error(t, err)
	})
}

func Test_DeleteKeyMaterial(t *testing.T) {
	tmpFile, err := createTestFile(t)
	require.NoError(t, err)
	//nolint:errcheck
	defer os.Remove(tmpFile.Name())

	ls := NewFileStorageManager(tmpFile.Name())
	ctx := context.Background()

	t.Run("should delete key material", func(t *testing.T) {
		did := randomDID(t)
		privateKey := "9d7abdd5a43573ab9b623c50b9fc8f4357329d3009fe0fc22c8931161d98a03d"
		id := getKeyID(&did, KeyTypeBabyJubJub, "BJJ:2290140c920a31a596937095f18a9ae15c1fe7091091be485f353968a4310380")

		err = ls.SaveKeyMaterial(ctx, map[string]string{
			jsonKeyType: string(KeyTypeBabyJubJub),
			jsonKeyData: privateKey,
		}, id)
		assert.NoError(t, err)

		keyMaterial, err := ls.getKeyMaterial(ctx, KeyID{
			Type: babyjubjub,
			ID:   id,
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			jsonKeyType: string(babyjubjub),
			jsonKeyData: privateKey,
		}, keyMaterial)

		err = ls.deleteKeyMaterial(ctx, KeyID{
			Type: babyjubjub,
			ID:   id,
		})

		require.NoError(t, err)

		_, err = ls.getKeyMaterial(ctx, KeyID{
			Type: babyjubjub,
			ID:   id,
		})
		require.Error(t, err)
	})
}

func createTestFile(t *testing.T) (*os.File, error) {
	t.Helper()
	tmpFile, err := os.Create("./kms.json")
	assert.NoError(t, err)
	initFileContent := []byte("[]")
	_, err = tmpFile.Write(initFileContent)
	assert.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	return tmpFile, err
}
