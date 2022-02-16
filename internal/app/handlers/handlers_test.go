package handlers

import (
	"bytes"
	"encoding/json"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/config"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/repository"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/wpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"runtime"
	"time"
	"fmt"
)

func testRequest(t *testing.T, config *config.Config, repo *repository.Repo, wp *wpool.WorkerPool, method, path, body string) (*http.Response, string) {

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
	workersCounter := runtime.NumCPU()

	wp := wpool.New(workersCounter)

	now := time.Now().Unix()

	if err != nil {
		log.Fatalf("failed to init repository:+%v", err)
	}

	
	newQuery := repository.LoginData{Login: fmt.Sprintf("test_%v", now), Password: "test"}
	inputBuf := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _ := testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String())	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()

	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String())	
	assert.Equal(t, http.StatusConflict, result.StatusCode)
	defer result.Body.Close()


	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/login", inputBuf.String())	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()


	newQuery = repository.LoginData{Login: fmt.Sprintf("test_%v", now), Password: "wrong"}
	inputBuf = bytes.NewBuffer([]byte{})
	if err = json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _ = testRequest(t, config, repo, wp, "POST", "/api/user/login", inputBuf.String())	
	assert.Equal(t, http.StatusUnauthorized, result.StatusCode)
	defer result.Body.Close()

}

