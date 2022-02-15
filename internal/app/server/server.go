package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/handlers"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/repository"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/wpool"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

type contextKey string

type Server interface {
	configureRouter() *chi.Mux
}

type srv struct {
	address    string
	AccrualURL string
	repo       repository.Repositorier
	wp 		   wpool.WorkerPooler
}

type gzipWriter struct {
	http.ResponseWriter
	Writer io.Writer
}

func (w gzipWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func New(address string, AccrualURL string, repo repository.Repositorier, wp wpool.WorkerPooler) *srv {
	server := &srv{
		address: address,
		AccrualURL: AccrualURL,
		repo:    repo,
		wp: 	 wp,
	}

	return server
}

func (s *srv) Run(ctx context.Context) (err error) {

	ctx, cancel := context.WithCancel(ctx)

	go s.wp.Run(ctx);

	router := s.ConfigureRouter()
	serv := &http.Server{
		Addr:    s.address,
		Handler: router,
	}

		
	go func() {
		if err := serv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("listener failed:+%v\n", err)
			cancel()
		}

	}()

	<-ctx.Done()

	log.Print("Server Started")

	ctxShutDown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer func() {
		// extra handling here
		cancel()
	}()

	if err := serv.Shutdown(ctxShutDown); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Print("Server Exited Properly")

	return

}

func (s *srv) ConfigureRouter() *chi.Mux {
	router := chi.NewRouter()
	
	router.Group(func(router chi.Router) {
		router.Use(middleware.Logger)
		router.Use(GzipHandle)
		
		router.Post("/api/user/register", func(rw http.ResponseWriter, r *http.Request) {
		 	handlers.RegisterHandler(s.repo)(rw, r)
		})

		router.Post("/api/user/login", func(rw http.ResponseWriter, r *http.Request) {
		 	handlers.LoginHandler(s.repo)(rw, r)
		})
	})

	router.Group(func(router chi.Router) {
		router.Use(middleware.Logger)
		router.Use(GzipHandle)
		router.Use(CheckUser)

		router.Get("/api/user/balance", func(rw http.ResponseWriter, r *http.Request) {
		 	u := r.Context().Value(contextKey("user_token")).(string)
		 	handlers.GetBalanceHandler(s.repo, u)(rw, r)
		})

		router.Get("/api/user/balance/withdrawals", func(rw http.ResponseWriter, r *http.Request) {
		 	u := r.Context().Value(contextKey("user_token")).(string)
		 	handlers.WithdrawListHandler(s.repo, u)(rw, r)
		})

		router.Get("/api/user/orders", func(rw http.ResponseWriter, r *http.Request) {
		 	u := r.Context().Value(contextKey("user_token")).(string)
		 	handlers.OrderListHandler(s.repo, u)(rw, r)
		})

		router.Post("/api/user/balance/withdraw", func(rw http.ResponseWriter, r *http.Request) {
		 	u := r.Context().Value(contextKey("user_token")).(string)
		 	handlers.WithdrawHandler(s.repo, u)(rw, r)
		})

		router.Post("/api/user/orders", func(rw http.ResponseWriter, r *http.Request) {
		 	u := r.Context().Value(contextKey("user_token")).(string)
		 	handlers.OrderHandler(s.repo, s.wp, s.AccrualURL, u)(rw, r)
		})

	})
		
	return router
}

func GzipHandle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			io.WriteString(w, err.Error())
			return
		}
		defer gz.Close()

		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			reader, err := gzip.NewReader(r.Body)

			if err != nil {
				io.WriteString(w, err.Error())
				return
			}

			defer reader.Close()

			b, err := ioutil.ReadAll(reader)

			if err != nil {
				io.WriteString(w, err.Error())
				return
			}

			r.Body = ioutil.NopCloser(bytes.NewBuffer(b))
		}

		w.Header().Set("Content-Encoding", "gzip")
		next.ServeHTTP(gzipWriter{ResponseWriter: w, Writer: gz}, r)
	})
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

			h := hmac.New(sha256.New, auth.SecretKey)
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