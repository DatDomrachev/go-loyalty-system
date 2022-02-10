package handlers

import (
	"bytes"
	"encoding/json"
	"github.com/DatDomrachev/shortner_go/internal/app/config"
	"github.com/DatDomrachev/shortner_go/internal/app/repository"
	"github.com/DatDomrachev/shortner_go/internal/app/wpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"runtime"
)

func testRequest(t *testing.T, config *config.Config, repo *repository.Repo, wp *wpool.WorkerPool, method, path, body string) (*http.Response, string) {

	request := httptest.NewRequest(method, path, nil)

	if body != "" {
		var content = []byte(body)
		reqContent := bytes.NewBuffer(content)
		request = httptest.NewRequest(method, path, reqContent)
	}

	//result, err := http.DefaultClient.Do(request)
	w := httptest.NewRecorder()

	if method == "POST" && path == "/" {
		SimpleWriteHandler(repo, config.BaseURL, "1")(w, request)
	}

	if method == "POST" && path == "/api/shorten" {
		SimpleJSONHandler(repo, config.BaseURL, "1")(w, request)
	}

	if method == "GET" {
		SimpleReadHandler(repo)(w, request)
	}

	if method == "DELETE" {
		DeleteItemsHandler(repo, wp, config.BaseURL, "1")(w, request)
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

	repo, err := repository.New(config.StoragePath, "")
	workersCounter := runtime.NumCPU()

	wp := wpool.New(workersCounter);

	if err != nil {
		log.Fatalf("failed to init repository:+%v", err)
	}
	result1, body1 := testRequest(t, config, repo, wp, "POST", "/", "http://google.com")
	assert.Equal(t, 201, result1.StatusCode)
	assert.Equal(t, "application/json", result1.Header.Get("Content-Type"))
	assert.Equal(t, config.BaseURL+"/1", body1)
	defer result1.Body.Close()

	result2, body2 := testRequest(t, config, repo, wp, "GET", "/1", "")
	assert.Equal(t, 307, result2.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", result2.Header.Get("Content-Type"))
	assert.Equal(t, "http://google.com", result2.Header.Get("Location"))
	log.Println(body2)
	defer result2.Body.Close()

	result3, body3 := testRequest(t, config, repo, wp, "GET", "/aboba23", "")
	assert.Equal(t, http.StatusBadRequest, result3.StatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", result3.Header.Get("Content-Type"))
	log.Println(body3)
	defer result3.Body.Close()

	newQuery := repository.Item{FullURL: "http://youtube.com"}

	inputBuf := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}

	newResult := repository.Result{ShortURL: config.BaseURL + "/2"}
	outputBuf := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(outputBuf).Encode(newResult); err != nil {
		log.Println(err.Error())
		return
	}

	result4, body4 := testRequest(t, config, repo, wp, "POST", "/api/shorten", inputBuf.String())
	assert.Equal(t, 201, result4.StatusCode)
	assert.Equal(t, "application/json", result4.Header.Get("Content-Type"))
	assert.Equal(t, outputBuf.String(), body4)
	log.Println(body4)
	defer result4.Body.Close()

	var links []string
	links = append(links, config.BaseURL + "/1")
	links = append(links, config.BaseURL + "/2")
	
	inputBuf2 := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(inputBuf2).Encode(links); err != nil {
		log.Println(err.Error())
		return
	}
	result5, body5 := testRequest(t, config, repo, wp, "DELETE", "/api/user/urls", inputBuf2.String())
	assert.Equal(t, 202, result5.StatusCode)
	log.Println(body5)
	defer result5.Body.Close()

}
