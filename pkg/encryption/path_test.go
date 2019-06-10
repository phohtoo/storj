// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package encryption

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"storj.io/storj/pkg/storj"
)

func newBucketStore(bucket string, key storj.Key) *Store {
	store := NewStore()
	store.Add(storj.NewUnencryptedPath("").WithBucket(bucket), storj.NewEncryptedPath(""), key)
	return store
}

func TestEncryption(t *testing.T) {
	forAllCiphers(func(cipher storj.Cipher) {
		for i, rawPath := range []string{
			"",
			"/",
			"//",
			"file.txt",
			"file.txt/",
			"fold1/file.txt",
			"fold1/fold2/file.txt",
			"/fold1/fold2/fold3/file.txt",
		} {
			errTag := fmt.Sprintf("test:%d path:%q cipher:%v", i, rawPath, cipher)

			var key storj.Key
			copy(key[:], randData(storj.KeySize))
			store := newBucketStore("b", key)
			bucketPath := storj.NewUnencryptedPath(rawPath).WithBucket("b")

			encBucketPath, err := EncryptBucketPath(bucketPath, cipher, store)
			if !assert.NoError(t, err, errTag) {
				continue
			}

			decBucketPath, err := DecryptBucketPath(encBucketPath, cipher, store)
			if !assert.NoError(t, err, errTag) {
				continue
			}

			assert.Equal(t, rawPath, decBucketPath.Path().Raw(), errTag)
		}
	})
}

// func TestDeriveKey(t *testing.T) {
// 	forAllCiphers(func(cipher storj.Cipher) {
// 		for i, tt := range []struct {
// 			rawPath      string
// 			depth     int
// 			errString string
// 		}{
// 			{"fold1/fold2/fold3/file.txt", -1, "encryption error: negative depth"},
// 			{"fold1/fold2/fold3/file.txt", 0, ""},
// 			{"fold1/fold2/fold3/file.txt", 1, ""},
// 			{"fold1/fold2/fold3/file.txt", 2, ""},
// 			{"fold1/fold2/fold3/file.txt", 3, ""},
// 			{"fold1/fold2/fold3/file.txt", 4, ""},
// 			{"fold1/fold2/fold3/file.txt", 5, "encryption error: depth greater than path length"},
// 		} {
// 			errTag := fmt.Sprintf("%d. %+v", i, tt)

// 			key := new(storj.Key)
// 			copy(key[:], randData(storj.KeySize))

// 			encrypted, err := EncryptPath(tt.path, cipher, key)
// 			if !assert.NoError(t, err, errTag) {
// 				continue
// 			}

// 			derivedKey, err := DerivePathKey(tt.path, key, tt.depth)
// 			if tt.errString != "" {
// 				assert.EqualError(t, err, tt.errString, errTag)
// 				continue
// 			}
// 			if !assert.NoError(t, err, errTag) {
// 				continue
// 			}

// 			shared := storj.JoinPaths(storj.SplitPath(encrypted)[tt.depth:]...)
// 			decrypted, err := DecryptPath(shared, cipher, derivedKey)
// 			if !assert.NoError(t, err, errTag) {
// 				continue
// 			}

// 			expected := storj.JoinPaths(storj.SplitPath(tt.path)[tt.depth:]...)
// 			assert.Equal(t, expected, decrypted, errTag)
// 		}
// 	})
// }

func forAllCiphers(test func(cipher storj.Cipher)) {
	for _, cipher := range []storj.Cipher{
		storj.Unencrypted,
		storj.AESGCM,
		storj.SecretBox,
	} {
		test(cipher)
	}
}
