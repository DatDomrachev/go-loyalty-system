package handlers

import (
	"bytes"
	"encoding/json"
	"encoding/hex"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/repository"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/wpool"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/auth"
	"io/ioutil"
	"net/http"
	"errors"
	"strings"
	"context"
	"fmt"
	"time"
	"crypto/md5"
	"log"
)

type JobData struct {
	OrderID string
	AccrualURL string
	UserToken string
}

type ArgsError struct {
	Message string
}

type BadResponse struct {
	Message string
}

type TooManyRequests struct {
	Message string
}

type DBError struct {
	Message string
}


func (ae *ArgsError) Error() string {
	return fmt.Sprintf("%v", ae.Message)
}

func (br *BadResponse) Error() string {
	return fmt.Sprintf("%v", br.Message)
}

func (tmr *TooManyRequests) Error() string {
	return fmt.Sprintf("%v", tmr.Message)
}

func (dbe *DBError) Error() string {
	return fmt.Sprintf("%v", dbe.Message)
}

func validateLuhnOrderNumber(num string) bool {
	idx := len(num) - 1
	total := 0
	pos := 0
	for i := idx; i > -1; i-- {
		char := num[i]
		if char == ' ' {
			continue
		}
		if char < 48 || char > 57 {
			return false
		}
		digit := int(char - 48)
		if pos%2 == 0 {
			total += digit
		} else {
			switch digit {
			case 1, 2, 3, 4:
				total += digit << 1
			case 5, 6, 7, 8:
				total += (digit << 1) - 9
			case 9:
				total += digit
			}
		}
		pos++
	}
	return pos > 1 && total%10 == 0
}

func RegisterHandler(repo repository.Repositorier) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request)  {

		var loginData repository.LoginData

		if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		password:= md5.Sum([]byte(loginData.Password))
		userID, err := repo.SaveUser(r.Context(), loginData.Login, hex.EncodeToString(password[:]))

		if err != nil {
			var ce *repository.ConflictError

			if errors.As(err, &ce) {
				w.WriteHeader(http.StatusConflict)
				return
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} 


		token := auth.GetToken(userID);
		token, err = repo.SaveUserToken(r.Context(), userID, token)
		
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
				cookie := &http.Cookie {
				Name:  "user_token",
				Value: token,
			}
			http.SetCookie(w, cookie)
			w.WriteHeader(http.StatusOK)
		}
	}
}


func LoginHandler(repo repository.Repositorier) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		var loginData repository.LoginData

		if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		password:= md5.Sum([]byte(loginData.Password))
		userToken, err := repo.FindUser(r.Context(), loginData.Login, hex.EncodeToString(password[:]))

		if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
		} else {
			cookie := &http.Cookie {
				Name:  "user_token",
				Value: userToken,
			}
			http.SetCookie(w, cookie)
			w.WriteHeader(http.StatusOK)
		}
	}
}

func GetBalanceHandler(repo repository.Repositorier, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var result *repository.Balance

		result, err := repo.GetBalance(r.Context(), userToken)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)

		buf := bytes.NewBuffer([]byte{})
		if err := json.NewEncoder(buf).Encode(result); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(buf.Bytes())

	}
}


func WithdrawListHandler(repo repository.Repositorier, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		items, err := repo.GetWithdrawals(r.Context(), userToken)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(items) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)

		buf := bytes.NewBuffer([]byte{})
		if err := json.NewEncoder(buf).Encode(items); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(buf.Bytes())

	}
}

func OrderListHandler(repo repository.Repositorier, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		items, err := repo.GetOrders(r.Context(), userToken)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(items) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.WriteHeader(http.StatusOK)

		buf := bytes.NewBuffer([]byte{})
		if err := json.NewEncoder(buf).Encode(items); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(buf.Bytes())

	}
}


func WithdrawHandler(repo repository.Repositorier, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var withdraw repository.Withdraw

		if err := json.NewDecoder(r.Body).Decode(&withdraw); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		check := validateLuhnOrderNumber(withdraw.OrderID)
		if !check {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		err := repo.SaveWithdraw(r.Context(), withdraw.OrderID, withdraw.Points, userToken)

		if err != nil {
			var lpe *repository.LowPointsError

			if errors.As(err, &lpe) {
				w.WriteHeader(http.StatusPaymentRequired)
				return
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusOK)
		}
	}
}


func OrderHandler(repo repository.Repositorier, wp wpool.WorkerPooler, AccrualURL string, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		contentType := r.Header.Get("Content-type")
		
		if contentType != "text/plain" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		body, err := ioutil.ReadAll(r.Body)

		if err != nil {
			defer r.Body.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		number := string(body)

		check := validateLuhnOrderNumber(number)
		if !check {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		accrual, err := repo.FindOrderAccrual(r.Context(), number)
		
		if err == nil {
			if(accrual.UserToken == userToken) {
				w.WriteHeader(http.StatusOK)
				return
			} else {
				w.WriteHeader(http.StatusConflict)
				return
			}
		}

		err = repo.CreateOrder(r.Context(), number, userToken)
		
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		go ProcessOrder(r.Context(), repo, wp, AccrualURL, userToken, number)
	}
}
		
func ProcessOrder(ctx context.Context, repo repository.Repositorier, wp wpool.WorkerPooler, accrualURL string, userToken string, OrderID string) {
		 
	execFn := func(ctx context.Context, args interface{}) (interface{}, error) {		
		argVal, ok := args.(JobData)
	
		if !ok {
			return nil, &ArgsError {
				Message: "Bad arguments",
			}
		}

		return CheckOrder(ctx, repo, argVal.OrderID, argVal.UserToken, argVal.AccrualURL)
	}

	t := time.Now().Unix()

	job := wpool.Job {
		Descriptor: wpool.JobDescriptor{
			ID:       wpool.JobID(fmt.Sprintf("%v_%v", OrderID, t)),
			JType:    "PROCESSING",
			Metadata: nil,
		},
		ExecFn: execFn,
		Args:   JobData{
			OrderID:   OrderID,
			UserToken: userToken,
			AccrualURL: accrualURL,
		},
	}

	go wp.GenerateFrom(job)




	for {
		select {
			case r, ok := <-wp.Results():
				if !ok {
					continue
				}

			err := r.Err
			if err != nil {
				var tmr *TooManyRequests

				if errors.As(err, &tmr) {
					time.Sleep(60 * time.Second)
				} else {
					log.Print(err)
					go wp.BroadcastDone(true)
					break
				}

			} else {
				val := r.Value.(repository.ProcessingOrder)
				log.Print(val.Status)
				if val.Status == "PROCESSED" || val.Status == "INVALID" {
					go wp.BroadcastDone(true)
					break
				}	
			}		


			t = time.Now().Unix()

			job = wpool.Job {
				Descriptor: wpool.JobDescriptor{
					ID:       wpool.JobID(fmt.Sprintf("%v_%v", OrderID, t)),
					JType:    "PROCESSING",
					Metadata: nil,
				},
				ExecFn: execFn,
				Args:   JobData{
					OrderID:   OrderID,
					UserToken: userToken,
					AccrualURL: accrualURL,
				},
			}

			go wp.GenerateFrom(job)

		case <-wp.Done():
			log.Print("done")
			return
		//default:
		 	//log.Print("waiting")
		} 
	
	}
		
}


func CheckOrder	(ctx context.Context, repo repository.Repositorier, orderID string, userToken string, endpoint string) (interface{}, error) {
	
	var processingOrder repository.ProcessingOrder

	url := endpoint+"/api/orders/"+orderID
	
	payload := strings.NewReader("")

	req, err := http.NewRequest("GET", url, payload)
    if err != nil {
        log.Printf("Can't prepare request to accrual %v\n", err)
        return nil, &BadResponse{
    		Message: "Can't prepare request to accrual"+ orderID,
    	}
    }

    res, err := http.DefaultClient.Do(req)
    
    if err != nil {
        log.Printf("Can't do request to accrual %v\n", err)
        return nil, &BadResponse{
    		Message: "Unable to do request to accrual"+ orderID,
    	}
    }

    if res.StatusCode == http.StatusInternalServerError {
    	log.Printf("BadResponse %v\n", orderID)
    	return nil, &BadResponse{
    		Message: "BadResponse on order "+ orderID,
    	}
    }

    if res.StatusCode == http.StatusTooManyRequests {
    	log.Printf("TooManyRequest %v\n", orderID)
    	return nil, &TooManyRequests {
    		Message: "TooManyRequest on order " + orderID,
    	}
    }


    if err := json.NewDecoder(res.Body).Decode(&processingOrder); err != nil {
    	defer res.Body.Close()
		return nil, &BadResponse{
    		Message: "Unable to read response on order "+ orderID,
    	}
	}

	log.Printf("Accrual correct response %v\n", orderID)
	err = repo.UpdateOrder(ctx, processingOrder.OrderID, processingOrder.Status, processingOrder.Accrual, userToken);
	if err != nil {
        log.Printf("DB error %v\n", err)
        return nil, &DBError{
    		Message: "DB error on order "+ orderID,
    	}
    }

	return processingOrder, nil
}