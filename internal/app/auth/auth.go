package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/binary"
)


var SecretKey = []byte("secret key of My Castle")

func GetToken(userID int) (token string) {

	//userId
	src := make([]byte, 8)
	ID := uint64(userID)
	binary.LittleEndian.PutUint64(src, ID)

	//подпись
	h := hmac.New(sha256.New, SecretKey)
	h.Write(src)
	
	return hex.EncodeToString(src) + hex.EncodeToString(h.Sum(nil))
}
