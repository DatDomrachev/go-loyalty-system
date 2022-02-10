package handlers

import (
	"bytes"
	"encoding/json"
	"github.com/DatDomrachev/shortner_go/internal/app/repository"
	"github.com/DatDomrachev/shortner_go/internal/app/wpool"
	"io/ioutil"
	"net/http"
	"errors"
	"strings"
	"context"
	"fmt"
	"time"
)

type JobData struct {
	WhereIn   string
	UserToken string
}

type ArgsError struct {
	Message string
}


func (ae *ArgsError) Error() string {
	return fmt.Sprintf("%v", ae.Message)
}


func SimpleReadHandler(repo repository.Repositorier) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fullURL, err := repo.Load(r.Context(), r.URL.Path)

		if err != nil {
			var ge *repository.GoneError

			if errors.As(err, &ge) {
				http.Error(w, "Deleted", http.StatusGone)
				return
			} else {
				http.Error(w, "Not found", http.StatusBadRequest)
				return
			}
		}

		http.Redirect(w, r, fullURL, http.StatusTemporaryRedirect)
	}
}

func SimpleWriteHandler(repo repository.Repositorier, baseURL string, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		result, err := repo.Store(r.Context(), string(data), userToken)
		
		if err != nil {
			var ce *repository.ConflictError

			if errors.As(err, &ce) {
				w.WriteHeader(http.StatusConflict)
				result = ce.ConflictID
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusCreated)
		}

		
		resp := baseURL + "/" + result

		w.Write([]byte(resp))
	}
}

func SimpleJSONHandler(repo repository.Repositorier, baseURL string, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		var url repository.Item

		if err := json.NewDecoder(r.Body).Decode(&url); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		result, err := repo.Store(r.Context(), url.FullURL, userToken)

		if err != nil {
			var ce *repository.ConflictError

			if errors.As(err, &ce) {
				w.Header().Set("content-type", "application/json")
				w.WriteHeader(http.StatusConflict)
				result = ce.ConflictID
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusCreated)
		}


		newResult := repository.Result{ShortURL: baseURL + "/" + result}

		buf := bytes.NewBuffer([]byte{})
		if err := json.NewEncoder(buf).Encode(newResult); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(buf.Bytes())
	}
}

func AllMyURLSHandler(repo repository.Repositorier, baseURL string, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		items, err := repo.GetByUser(r.Context(), userToken)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(items) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		for i := range items {
			items[i].ShortURL = baseURL + "/" + items[i].ShortURL
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

func PingDB(repo repository.Repositorier) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		result := repo.PingDB(r.Context())

		if !result {
			http.Error(w, "No connection to DB", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	}
}

func BatchHandler(repo repository.Repositorier, baseURL string, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var items []repository.CorrelationItem

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = json.Unmarshal(body, &items)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		shortens, err := repo.BatchAll(r.Context(), items, userToken)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for i := range shortens {
			shortens[i].ShortURL = baseURL + "/" + shortens[i].ShortURL
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)

		buf := bytes.NewBuffer([]byte{})
		if err := json.NewEncoder(buf).Encode(shortens); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(buf.Bytes())

	}
}

func DeleteItemsHandler(repo repository.Repositorier, wp wpool.WorkerPooler, baseURL string, userToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var shortens []string
		var ids []string
	
		err = json.Unmarshal(body, &shortens)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return 
		}

		for i := range shortens {
			if len(shortens[i]) > 0 {
				ids = append(ids, strings.ReplaceAll(shortens[i], baseURL+"/", ""))
			}
		}

		if len(ids) > 0 {

			
			whereIn := strings.Join(ids, ",")

			execFn := func(ctx context.Context, args interface{}) (interface{}, error) {		
				argVal, ok := args.(JobData)
			
				if !ok {
					return nil, &ArgsError {
						Message: "Bad arguments",
					}
				}

				return repo.DeleteByUser(ctx, argVal.WhereIn, argVal.UserToken)
			}

			time := time.Now().Unix()

			job := wpool.Job {
				Descriptor: wpool.JobDescriptor{
					ID:       wpool.JobID(fmt.Sprintf("%v_%v", userToken, time)),
					JType:    "delete",
					Metadata: nil,
				},
				ExecFn: execFn,
				Args:   JobData{
					WhereIn:   whereIn,
					UserToken: userToken,
				},
			}

			go wp.GenerateFrom(job)
		}
		
		w.WriteHeader(http.StatusAccepted)

	}
}	