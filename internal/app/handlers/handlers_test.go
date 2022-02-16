package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"crypto/md5"
	"encoding/hex"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/config"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/repository"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/wpool"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ShiraazMoollatjie/goluhn"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"runtime"
	"time"
	"fmt"
)

func testRequest(t *testing.T, config *config.Config, repo *repository.Repo, wp *wpool.WorkerPool, method, path, body, token string) (*http.Response, string) {

	request := httptest.NewRequest(method, path, nil)

	if body != "" {
		var content = []byte(body)
		reqContent := bytes.NewBuffer(content)
		request = httptest.NewRequest(method, path, reqContent)
	}

	
	w := httptest.NewRecorder()

	if method == "POST" && path == "/api/user/register" {
		RegisterHandler(repo)(w, request)
	}

	if method == "POST" && path == "/api/user/login" {
		LoginHandler(repo)(w, request)
	}

	if method == "POST" && path == "/api/user/orders" {
		OrderHandler(repo, wp, "", token)(w, request)
	}


	result := w.Result()

	respBody, err := ioutil.ReadAll(result.Body)
	require.NoError(t, err)

	defer result.Body.Close()
	return result, string(respBody)

}


func TestRouter(t *testing.T) {

	config, err := config.New()
	config.InitFlags()
	if err != nil {
		log.Printf("failed to configurate:+%v\n", err)
	}

	repo, err := repository.New(config.DBURL)
	if err != nil {
		log.Fatalf("failed to init repo:+%v", err)
	}

	workersCounter := runtime.NumCPU()

	wp := wpool.New(workersCounter)

	now := time.Now().Unix()

	
	//рега
	newQuery := repository.LoginData{Login: fmt.Sprintf("test_%v", now), Password: "test"}
	inputBuf := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _ := testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String(), "")	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()


	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String(), "")	
	assert.Equal(t, http.StatusConflict, result.StatusCode)
	defer result.Body.Close()

	//логин
	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/login", inputBuf.String(), "")	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()

	password:= md5.Sum([]byte(newQuery.Password))
	userID, err := repo.SaveUser(context.Background(), newQuery.Login, hex.EncodeToString(password[:]))
	if err != nil {
	  log.Fatalf("failed to catch userID:+%v", err)
	}

	token := auth.GetToken(userID);

	newQuery = repository.LoginData{Login: fmt.Sprintf("test_%v", now), Password: "wrong"}
	inputBuf = bytes.NewBuffer([]byte{})
	if err = json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/login", inputBuf.String(), "")	
	assert.Equal(t, http.StatusUnauthorized, result.StatusCode)
	defer result.Body.Close()


	//отправка заказа
	n1 := goluhn.Generate(16)
	
	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/orders","1", token)	
	assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
	defer result.Body.Close()
	
	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/orders",n1, token)	
	assert.Equal(t, http.StatusAccepted, result.StatusCode)
	defer result.Body.Close()

	time.Sleep(10 * time.Second)

	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/orders",n1, token)	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()

	//рега2
	newQuery = repository.LoginData{Login: fmt.Sprintf("test__%v", now), Password: "test"}
	inputBuf = bytes.NewBuffer([]byte{})
	if err = json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String(), "")	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()

	password= md5.Sum([]byte(newQuery.Password))
	userID, err = repo.SaveUser(context.Background(), newQuery.Login, hex.EncodeToString(password[:]))
	if err != nil {
	  log.Fatalf("failed to catch userID second:+%v", err)
	}

	token2 := auth.GetToken(userID);

	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/orders",n1, token2)	
	assert.Equal(t, http.StatusConflict, result.StatusCode)
	defer result.Body.Close()

}
