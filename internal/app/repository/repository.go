package repository

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	_ "github.com/jackc/pgx/v4/stdlib"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"fmt"
	//"github.com/pressly/goose/v3"
)

type Repositorier interface {
	Load(ctx context.Context, shortURL string) (string, error)
	Store(ctx context.Context, url string, userToken string) (string, error)
	GetByUser(ctx context.Context, userToken string) ([]MyItem, error)
	BatchAll(ctx context.Context, items []CorrelationItem, userToken string) ([]CorrelationShort, error)
	PingDB(ctx context.Context) bool
	DeleteByUser(ctx context.Context, ids string, userToken string)(QueryResult, error)
}

type Item struct {
	FullURL   string `json:"url"`
	UserToken string `json:"user_token"`
}

type MyItem struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

type Result struct {
	ShortURL string `json:"result"`
}

type DataBase struct {
	conn *sql.DB
}

type Repo struct {
	StoragePath string
	items       []Item
	DB          *DataBase
}

type CorrelationItem struct {
	CorrectionalID string `json:"correlation_id"`
	OriginalURL    string `json:"original_url"`
}

type CorrelationShort struct {
	CorrectionalID string `json:"correlation_id"`
	ShortURL       string `json:"short_url"`
}

type ConflictError struct {
	ConflictID string
	Err error
}

type GoneError struct {
	Message string
}

type QueryResult struct {
	Message string 
}


func (ce *ConflictError) Error() string {
	return fmt.Sprintf("%v %v", ce.ConflictID, ce.Err)
}

func (ge *GoneError) Error() string {
	return fmt.Sprintf("%v", ge.Message)
}

func New(storagePath string, databaseURL string) (*Repo, error) {
	var items []Item
	dataBase := &DataBase{
		conn: nil,
	}

	repo := &Repo{
		StoragePath: storagePath,
		DB:          dataBase,
		items:       items,
	}

	if storagePath != "" {
		err := repo.readFromFile()

		if err != nil {
			return nil, err
		}
	}

	if databaseURL != "" {
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			db.Close()
			return nil, err
		}

		if err := db.Ping(); err != nil {
			db.Close()
			return nil, err
		}

		// Не взлетел гусь на автотестах, жаль
		// err = goose.Up(db, "migrations" )
		// if err != nil {
		// 	log.Fatalf("failed executing migrations: %v\n", err)
		// }

		_, err = db.Exec("CREATE TABLE if not exists url (id BIGSERIAL primary key, full_url text,user_token text)")

		if err != nil {
			return nil, err
		}

		_, err = db.Exec("ALTER TABLE url ADD COLUMN IF NOT EXISTS correlation_id text")

		if err != nil {
			return nil, err
		}

		_, err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS unique_urls_constrain ON url (full_url)")

		if err != nil {
			return nil, err
		}

		_, err = db.Exec("ALTER TABLE url ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT false")

		if err != nil {
			return nil, err
		}

		dataBase := &DataBase{
			conn: db,
		}

		repo.DB = dataBase
	}

	return repo, nil
}

func (r *Repo) GetByUser(ctx context.Context, user string) ([]MyItem, error) {

	var myItems []MyItem

	if r.DB.conn != nil {

		
		rows, err := r.DB.conn.QueryContext(ctx, "Select id::varchar(255), full_url from url WHERE user_token = $1", user)

		if err != nil {
			return myItems, err
		}

		for rows.Next() {
			var item MyItem
			err = rows.Scan(&item.ShortURL, &item.OriginalURL)

			if err != nil {
				return myItems, err
			}

			myItems = append(myItems, item)
		}

		err = rows.Err()
		if err != nil {
			return myItems, err
		}

		return myItems, nil

	}

	for i := range r.items {
		if user == r.items[i].UserToken {
			myItem := MyItem{
				ShortURL:    strconv.Itoa(i + 1),
				OriginalURL: r.items[i].FullURL,
			}
			myItems = append(myItems, myItem)
		}
	}

	return myItems, nil
}

func (r *Repo) Load(ctx context.Context, shortURL string) (string, error) {

	fullURL := ""
	isDeleted := false

	param := strings.TrimPrefix(shortURL, `/`)

	id, err := strconv.Atoi(param)

	if err != nil {
		return fullURL, err
	}

	for i := range r.items {
		if i == id-1 {
			fullURL = r.items[i].FullURL
			break
		}
	}

	if r.DB.conn != nil { 
		row := r.DB.conn.QueryRowContext(ctx, "SELECT full_url, is_deleted from url WHERE id = $1", id)
		err := row.Scan(&fullURL, &isDeleted)
		if err != nil {
			log.Print(err.Error())
			return "", err
		}

		if isDeleted {
			log.Print("wasDeleted")
			return "", &GoneError {
				Message: "Url was deleted",
			}
		}

	}

	return fullURL, nil
}

func (r *Repo) Store(ctx context.Context, url string, userToken string) (string, error) {

	newItem := Item{FullURL: url, UserToken: userToken}
	r.items = append(r.items, newItem)
	result := len(r.items)

	if r.StoragePath != "" {

		err := r.writeToFile(newItem)

		if err != nil {
			return "", err
		}

	}

	if r.DB.conn != nil {
		row := r.DB.conn.QueryRowContext(ctx, "Insert into url (full_url, user_token) VALUES ($1, $2) RETURNING id", url, userToken)
		err := row.Scan(&result)

		if err != nil {
			err, ok := err.(*pgconn.PgError)

			if ok && err.Code == pgerrcode.UniqueViolation {
				row = r.DB.conn.QueryRowContext(ctx, "SELECT id from url WHERE full_url = $1", url)
				row.Scan(&result)
				return "", &ConflictError {
					ConflictID: strconv.Itoa(result),
					Err: err,
				}
			} else {
				return "", err
			}
		}
	}

	return strconv.Itoa(result), nil
}

func (r *Repo) readFromFile() error {
	file, err := os.OpenFile(r.StoragePath, os.O_RDONLY|os.O_CREATE, 0777)

	if err != nil {
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {

		data := scanner.Bytes()

		item := Item{}
		err := json.Unmarshal(data, &item)

		if err != nil {
			return err
		}

		r.items = append(r.items, item)

	}

	return nil
}

func (r *Repo) writeToFile(newItem Item) error {

	data, err := json.Marshal(&newItem)

	if err != nil {
		return err
	}

	file, err := os.OpenFile(r.StoragePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0777)

	if err != nil {
		return err
	}

	defer file.Close()

	writer := bufio.NewWriter(file)

	if _, err := writer.Write(data); err != nil {
		return err
	}

	if err := writer.WriteByte('\n'); err != nil {
		return err
	}

	return writer.Flush()

}

func (r *Repo) PingDB(ctx context.Context) bool {
	if r.DB.conn == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := r.DB.conn.PingContext(ctx)
	if err != nil {
		log.Print(err.Error())
		return false
	}

	return true
}

func (r *Repo) BatchAll(ctx context.Context, items []CorrelationItem, userToken string) ([]CorrelationShort, error) {

	var shortens []CorrelationShort

	id := 0

	for _, i := range items {
		row := r.DB.conn.QueryRowContext(ctx, "Insert into url (full_url, user_token, correlation_id) VALUES($1,$2,$3) ON CONFLICT(full_url) DO UPDATE SET full_url=EXCLUDED.full_url RETURNING id", i.OriginalURL, userToken, i.CorrectionalID)
		err := row.Scan(&id)
		if err != nil {
			return nil, err
		}

		shorten := CorrelationShort{
			CorrectionalID: i.CorrectionalID,
			ShortURL:       strconv.Itoa(id),
		}

		shortens = append(shortens, shorten)

	}

	return shortens, nil
}

func (r *Repo) DeleteByUser(ctx context.Context, ids string, userToken string) (QueryResult, error) {
	
	_, err := r.DB.conn.ExecContext(ctx, "UPDATE url SET is_deleted = true WHERE user_token = $1 AND id IN ("+ ids+")", userToken)

	if err != nil {
		log.Print(err.Error())
		return QueryResult{ Message: "fail" }, err
	}
		
	return QueryResult{ Message: "done" } ,nil

}
