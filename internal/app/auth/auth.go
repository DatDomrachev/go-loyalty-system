package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)


var secretKey = []byte("secret key of My Castle")

func GetToken(userId int) (cookie *http.Cookie) {

	//userId
	src := make([]byte, 8)
	binary.LittleEndian.PutUint64(src, userId)

	//подпись
	h := hmac.New(sha256.New, secretKey)
	h.Write(src)
	
	return hex.EncodeToString(src) + hex.EncodeToString(h.Sum(nil))
}

func CheckUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		

		cookie, err := r.Cookie("user_token")

		if err == nil {

			userKey := cookie.Value

			data, err := hex.DecodeString(userKey)

			if err != nil {
				log.Fatalf("CookieRead error:%+v", err)
			}

			h := hmac.New(sha256.New, secretKey)
			h.Write(data[:8])
			sign := h.Sum(nil)

			if !hmac.Equal(sign, data[8:]) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}

		

		ctx := context.WithValue(r.Context(), contextKey("user_token"), cookie.Value)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}