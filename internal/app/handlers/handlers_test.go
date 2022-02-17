package handlers

import (
	"bytes"
	"encoding/json"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/config"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/repository"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/wpool"
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
	"github.com/golang-module/carbon/v2"
)

func testRequest(t *testing.T, config *config.Config, repo *repository.Repo, wp *wpool.WorkerPool, method, path, body, token string, textFlag bool) (*http.Response, string, []*http.Cookie) {

	request := httptest.NewRequest(method, path, nil)

	if body != "" {
		var content = []byte(body)
		reqContent := bytes.NewBuffer(content)
		request = httptest.NewRequest(method, path, reqContent)
	}

	
	w := httptest.NewRecorder()
	if textFlag {
		request.Header.Set("Content-type", "text/plain")
	}

	if method == "POST" && path == "/api/user/register" {
		RegisterHandler(repo)(w, request)
	}

	if method == "POST" && path == "/api/user/login" {
		LoginHandler(repo)(w, request)
	}

	if method == "POST" && path == "/api/user/orders" {
		OrderHandler(repo, wp, "", token)(w, request)
	}

	if method == "GET" && path == "/api/user/balance" {
		GetBalanceHandler(repo, token)(w, request)
	}

	if method == "POST" && path == "/api/user/balance/withdraw" {
		WithdrawHandler(repo, token)(w, request)
	}

	if method == "GET" && path == "/api/user/orders" {
		OrderListHandler(repo, token)(w, request)
	}

	if method == "GET" && path == "/api/user/balance/withdrawals" {
		WithdrawListHandler(repo, token)(w, request)
	}

	cookies := w.Result().Cookies()
	result := w.Result()

	respBody, err := ioutil.ReadAll(result.Body)
	require.NoError(t, err)

	defer result.Body.Close()
	return result, string(respBody), cookies

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

	timeUnix := time.Now().Unix()
	
	login := fmt.Sprintf("test_%v", timeUnix)

	//рега
	newQuery := repository.LoginData{Login: login, Password: "test"}
	inputBuf := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _,_ := testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String(), "", false)	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()


	result,_,_ = testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String(), "", false)	
	assert.Equal(t, http.StatusConflict, result.StatusCode)
	defer result.Body.Close()

	//логин
	result, _, cookies := testRequest(t, config, repo, wp, "POST", "/api/user/login", inputBuf.String(), "", false)	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()

	token:= cookies[0].Value    

	newQuery = repository.LoginData{Login: login, Password: "wrong"}
	inputBuf = bytes.NewBuffer([]byte{})
	if err = json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _,_ = testRequest(t, config, repo, wp, "POST", "/api/user/login", inputBuf.String(), "", false)	
	assert.Equal(t, http.StatusUnauthorized, result.StatusCode)
	defer result.Body.Close()


	//отправка заказа
	n1 := goluhn.Generate(16)
	n2 := goluhn.Generate(16)
	
	result, _,_ = testRequest(t, config, repo, wp, "POST", "/api/user/orders", "01", token, true)	
	assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
	defer result.Body.Close()

	result, _,_ = testRequest(t, config, repo, wp, "POST", "/api/user/orders",n1, token, true)	
	assert.Equal(t, http.StatusAccepted, result.StatusCode)
	defer result.Body.Close()
	timeString1 := carbon.Time2Carbon(time.Now()).ToRfc3339String()

	result, _,_ = testRequest(t, config, repo, wp, "POST", "/api/user/orders", n1, token, true)	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()

	//рега2
	login2 := fmt.Sprintf("test__%v", timeUnix)

	newQuery = repository.LoginData{Login: login2, Password: "test"}
	inputBuf = bytes.NewBuffer([]byte{})
	if err = json.NewEncoder(inputBuf).Encode(newQuery); err != nil {
		log.Println(err.Error())
		return
	}
	result, _,cookies = testRequest(t, config, repo, wp, "POST", "/api/user/register", inputBuf.String(), "", false)	
	assert.Equal(t, 200, result.StatusCode)
	defer result.Body.Close()

	
	token2:= cookies[0].Value 

	result, _,_ = testRequest(t, config, repo, wp, "POST", "/api/user/orders", n1, token2, true)	
	assert.Equal(t, http.StatusConflict, result.StatusCode)
	defer result.Body.Close()


	result, _,_ = testRequest(t, config, repo, wp, "POST", "/api/user/orders", n2, token, true)	
	assert.Equal(t, http.StatusAccepted, result.StatusCode)
	defer result.Body.Close()
	timeString2 := carbon.Time2Carbon(time.Now()).ToRfc3339String()

	//баланс
	newResult := repository.Balance{Current: 0, Withdrawn: 0}
	outputBuf := bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(outputBuf).Encode(newResult); err != nil {
		log.Println(err.Error())
		return
	}
	result, body,_ := testRequest(t, config, repo, wp, "GET", "/api/user/balance", "", token, false)	
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "application/json", result.Header.Get("Content-Type"))
	assert.Equal(t, outputBuf.String(), body)
	defer result.Body.Close()


	//списание
	newQueryW := repository.Withdraw{OrderID: "1", Points: 5}
	inputBuf = bytes.NewBuffer([]byte{})
	if err = json.NewEncoder(inputBuf).Encode(newQueryW); err != nil {
		log.Println(err.Error())
		return
	}
	result,_,_ = testRequest(t, config, repo, wp, "POST", "/api/user/balance/withdraw", inputBuf.String(), token, false)	
	assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
	defer result.Body.Close()

	newQueryW = repository.Withdraw{OrderID: n1, Points: 5}
	inputBuf = bytes.NewBuffer([]byte{})
	if err = json.NewEncoder(inputBuf).Encode(newQueryW); err != nil {
		log.Println(err.Error())
		return
	}
	result,_,_ = testRequest(t, config, repo, wp, "POST", "/api/user/balance/withdraw", inputBuf.String(), token, false)	
	assert.Equal(t, http.StatusPaymentRequired, result.StatusCode)
	defer result.Body.Close()


	//списания
	result,_,_ = testRequest(t, config, repo, wp, "GET", "/api/user/balance/withdrawals", "", token, false)	
	assert.Equal(t, http.StatusNoContent, result.StatusCode)
	defer result.Body.Close()


	//начисления
	var accurals []repository.Accrual
	
	accural1 := repository.Accrual{OrderID: n1, Status: "NEW", Accrual:0, UploadedAt:timeString1}
	accural2 := repository.Accrual{OrderID: n2, Status: "NEW", Accrual:0, UploadedAt:timeString2}

	accurals = append(accurals, accural1)
	accurals = append(accurals, accural2)

	outputBuf = bytes.NewBuffer([]byte{})
	if err := json.NewEncoder(outputBuf).Encode(accurals); err != nil {
		log.Println(err.Error())
		return
	}
	
	result, body,_ = testRequest(t, config, repo, wp, "GET", "/api/user/orders", "", token, false)	
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "application/json", result.Header.Get("Content-Type"))
	assert.Equal(t, outputBuf.String(), body)
	defer result.Body.Close()


	result,_,_ = testRequest(t, config, repo, wp, "GET", "/api/user/orders", "", token2, false)	
	assert.Equal(t, http.StatusNoContent, result.StatusCode)
	defer result.Body.Close()

	
}
