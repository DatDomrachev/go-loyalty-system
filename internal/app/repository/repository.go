package repository

import (
	"context"
	"database/sql"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	_ "github.com/jackc/pgx/v4/stdlib"
	"log"
	"fmt"
	"time"
	"github.com/golang-module/carbon/v2"
	//"github.com/pressly/goose/v3"
)

type Repositorier interface {
	SaveUser(ctx context.Context, login string, password string) (int, error)
	SaveUserToken(ctx context.Context, id int, userToken string) (string, error)
	FindUser(ctx context.Context, login string, password string) (string, error)
	GetBalance(ctx context.Context, userToken string) (*Balance, error)
	GetWithdrawals(ctx context.Context, userToken string) ([]ProcessedWithdraw, error)
	GetOrders(ctx context.Context, userToken string) ([]Accrual, error)
	SaveWithdraw(ctx context.Context, orderID string, points float64, userToken string) (error)
	CreateOrder(ctx context.Context, orderID string, userToken string) (error)
	UpdateOrder(ctx context.Context, orderID string, status string, accrual float64, userToken string) (error)
	FindOrderAccrual(ctx context.Context, orderID string) (*AccrualRaw, error)
}

const TypeAccrual = 1
const TypeWithdraw = 2

const StatusNew = 1
const StatusProcessing = 2
const StatusInvalid = 3
const StatusProcessed = 4

type LoginData struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type AccrualRaw struct {
	UserToken  string  `json:"token"` 
	OrderID    string  `json:"number"`
	Status     int     `json:"status"`
	Accrual    float64 `json:"accrual"`
	UploadedAt string  `json:"uploaded_at"`
}

type Accrual struct {
	OrderID    string  `json:"number"`
	Status     string  `json:"status"`
	Accrual    float64 `json:"accrual,omitempty"`
	UploadedAt string  `json:"uploaded_at"`
}

type Balance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type Withdraw struct {
	OrderID string 	`json:"order"`
	Points 	float64 `json:"sum"`
}

type ProcessedWithdraw struct {
	OrderID 	string 	`json:"order"`
	Points 		float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}

type ProcessingOrder struct {
	OrderID    string  `json:"number"`
	Status     string  `json:"status"`
	Accrual    float64 `json:"accrual" default:"0.0"`
}


type DataBase struct {
	conn *sql.DB
}

type Repo struct {
	StoragePath string
	DB          *DataBase
}


type ConflictError struct {
	Err error
}

type DBError struct {
	Message string
}

type LowPointsError struct {
	Message string
}

type QueryResult struct {
	Message string 
}

func (lpe *LowPointsError) Error() string {
	return fmt.Sprintf("%v", lpe.Message)
}

func (ce *ConflictError) Error() string {
	return fmt.Sprintf("%v", ce.Err)
}

func (dbe *DBError) Error() string {
	return fmt.Sprintf("%v", dbe.Message)
}

var insertTransaction *sql.Stmt
var updateTransaction *sql.Stmt
var updateBalance *sql.Stmt

func getStatusMap() map[int]string {
	return map[int]string{
		StatusNew: "NEW" ,
		StatusProcessing: "PROCESSING",
		StatusInvalid: "INVALID",
		StatusProcessed: "PROCESSED",

	}
}

func firstKeyByValue(m map[int]string, value string) int {
	for k, v := range m {
		if value == v {
			return k
		}
	}
	return 0
}

func New(dataBaseURL string) (*Repo, error) {
	
	dataBase := &DataBase{
		conn: nil,
	}

	repo := &Repo{
		DB: dataBase,
	}

	
	if dataBaseURL != "" {
		db, err := sql.Open("pgx", dataBaseURL)
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

		_, err = db.Exec("CREATE TABLE if not exists users (id BIGSERIAL primary key, login text, password text, user_token text, balance float default 0.0, withdrawn float default 0.0)")

		if err != nil {
			return nil, err
		}

		
		_, err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS unique_login_constrain ON users(login)")

		if err != nil {
			return nil, err
		}

		_, err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS unique_token_constrain ON users(user_token)")

		if err != nil {
			return nil, err
		}


		_, err = db.Exec("CREATE TABLE if not exists transactions (id BIGSERIAL primary key, user_token text, order_id text, type integer, status integer, points float default 0.0, uploaded_at TIMESTAMPTZ default now(), processed_at TIMESTAMPTZ, FOREIGN KEY (user_token) REFERENCES users (user_token))")

		if err != nil {
			return nil, err
		}


		insertTransaction, err = db.Prepare("INSERT INTO transactions (user_token, order_id, type, status, points, processed_at) VALUES($1,$2,$3,$4,$5,$6)") 
		if err != nil {
			return nil, err
		}

		updateTransaction, err = db.Prepare("UPDATE transactions set status = $1, points = $2, processed_at = $3 where order_id = $4 and type = $5") 
		if err != nil {
			return nil, err
		}

		updateBalance, err = db.Prepare("UPDATE users set balance = $1, withdrawn = $2 where user_token = $3") 
		if err != nil {
			return nil, err
		}

		dataBase := &DataBase{
			conn: db,
		}

		repo.DB = dataBase

	} else {
		return nil, &DBError {
			Message: "Подключение к БД отсутствует",
		} 
	}

	return repo, nil
}



func (r *Repo) SaveUser(ctx context.Context, login string, password string) (int, error) {
	id := 0

	row := r.DB.conn.QueryRowContext(ctx, "Insert into users (login, password) VALUES ($1, $2) RETURNING id", login, password)
	err := row.Scan(&id)

	if err != nil {
		err, ok := err.(*pgconn.PgError)

		if ok && err.Code == pgerrcode.UniqueViolation {
			return 0, &ConflictError {
				Err: err,
			}
		} else {
			return 0, err
		}
	}

	return id, nil
}



func (r *Repo) SaveUserToken(ctx context.Context, id int, userToken string) (string, error) {
	_, err := r.DB.conn.ExecContext(ctx, "UPDATE users SET user_token = $1 WHERE id = $2", userToken, id)

	if err != nil {
		return "", err
	}
		
	return userToken, nil

}


func (r *Repo) FindUser(ctx context.Context, login string, password string) (string, error) {
	token := ""
	row := r.DB.conn.QueryRowContext(ctx, "SELECT user_token from users WHERE login = $1 and password = $2", login, password)
	err := row.Scan(&token)
	if err != nil {
		log.Print(err.Error())
		return "", err
	}
	return token, nil

}

func (r *Repo) GetBalance(ctx context.Context, userToken string) (*Balance, error) {
	balance := 0.0
	withdrawn := 0.0
	row := r.DB.conn.QueryRowContext(ctx, "SELECT balance, withdrawn from users WHERE user_token = $1", userToken)
	err := row.Scan(&balance, &withdrawn)
	if err != nil {
		log.Print(err.Error())
		return &Balance{
			Current: balance,
			Withdrawn: withdrawn,
		}, err
	}


	return &Balance{
		Current: balance,
		Withdrawn: withdrawn,
	}, nil

}


func (r *Repo) GetWithdrawals(ctx context.Context, userToken string) ([]ProcessedWithdraw, error) {

	var myWithdraws []ProcessedWithdraw

	rows, err := r.DB.conn.QueryContext(ctx, "Select order_id, points, processed_at from transactions WHERE user_token = $1 AND type = $2 AND status = $3 ORDER BY processed_at", userToken, TypeWithdraw, StatusProcessed)

	if err != nil {
		return myWithdraws, err
	}

	for rows.Next() {
		var item ProcessedWithdraw
		err = rows.Scan(&item.OrderID, &item.Points, &item.ProcessedAt)

		if err != nil {
			return myWithdraws, err
		}

		myWithdraws = append(myWithdraws, item)
	}

	err = rows.Err()
	if err != nil {
		return myWithdraws, err
	}

	return myWithdraws, nil

}


func (r *Repo) GetOrders(ctx context.Context, userToken string) ([]Accrual, error) {

	var myAccruals []Accrual

	m := getStatusMap()

	rows, err := r.DB.conn.QueryContext(ctx, "Select user_token, order_id, status, points, uploaded_at from transactions WHERE user_token = $1 AND type = $1 ORDER BY uploaded_at", userToken, TypeAccrual)

	if err != nil {
		return myAccruals, err
	}

	for rows.Next() {
		var itemRaw AccrualRaw
		var item Accrual
		err = rows.Scan(&itemRaw.UserToken, &itemRaw.OrderID, &itemRaw.Status, &itemRaw.Accrual, &itemRaw.UploadedAt)

		if err != nil {
			return myAccruals, err
		}

		item.OrderID = itemRaw.OrderID
		item.Status = m[itemRaw.Status]
		
		if itemRaw.Accrual > 0 {	
			item.Accrual = itemRaw.Accrual
		}
		
		item.UploadedAt = itemRaw.UploadedAt 
		
		myAccruals = append(myAccruals, item)
	}

	err = rows.Err()
	if err != nil {
		return myAccruals, err
	}

	return myAccruals, nil

}

func (r *Repo) SaveWithdraw(ctx context.Context, orderID string, points float64, userToken string) (error) {
	
	balance, err := r.GetBalance(ctx, userToken)
	
	if err != nil {
		return err
	}

	if points > balance.Current {
		return &LowPointsError {
			Message: "Недостаточно баллов для списания",
		}
	}

	newBalance := balance.Current - points
	newWithdrawn := balance.Withdrawn + points
	timeString := carbon.Time2Carbon(time.Now()).ToRfc3339String()

	tx, err := r.DB.conn.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()


    txStmt := tx.StmtContext(ctx, insertTransaction)
    
    if _, err = txStmt.ExecContext(ctx, userToken, orderID, TypeWithdraw, StatusProcessed, points, timeString); err != nil {
        return err
    }
    
    txStmt = tx.StmtContext(ctx, updateBalance)
    if _, err = txStmt.ExecContext(ctx, newBalance, newWithdrawn, userToken); err != nil {
        return err
    }

    return tx.Commit()
	
}


func (r *Repo) FindOrderAccrual(ctx context.Context, orderID string) (*AccrualRaw, error) {
	token := ""
	status := 0
	points := 0.0
	uploadedAt := "NULL"

	row := r.DB.conn.QueryRowContext(ctx, "SELECT user_token, status, points, uploaded_at from transactions WHERE order_id = $1 and type = $2", orderID, TypeAccrual)
	err := row.Scan(&token, &status, &points, &uploadedAt)
	if err != nil {
		return nil, err
	}

	return &AccrualRaw{
		UserToken: token,
		OrderID: orderID,
		Status: status,
		Accrual: points,
		UploadedAt: uploadedAt,
	}, nil

}


func (r *Repo) CreateOrder(ctx context.Context, orderID string, userToken string) (error) {

	tx, err := r.DB.conn.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    txStmt := tx.StmtContext(ctx, insertTransaction)
    
    if _, err = txStmt.ExecContext(ctx, userToken, orderID, TypeAccrual, StatusNew, 0.0, "NULL"); err != nil {
        return err
    }
    
    return tx.Commit()

}

func (r *Repo) UpdateOrder(ctx context.Context, orderID string, status string, accrual float64, userToken string) (error) {
	m := getStatusMap()
	statusKey := firstKeyByValue(m, status)
	
	if(statusKey == 0) {
		//r.CreateOrder(ctx, orderID, userToken)
		return nil
	}

	newBalance := 0.0
	withdrawn := 0.0
	if statusKey == StatusProcessed {
    	balance, err := r.GetBalance(ctx, userToken)
	
		if err != nil {
			return err
		}
		newBalance = balance.Current + accrual
		withdrawn = balance.Withdrawn
    }

	tx, err := r.DB.conn.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    timeString := "NULL"

    if statusKey == StatusProcessed {
    	timeString = carbon.Time2Carbon(time.Now()).ToRfc3339String()
    }

    txStmt := tx.StmtContext(ctx, updateTransaction)
    
    if _, err = txStmt.ExecContext(ctx, statusKey, accrual, timeString, orderID, TypeAccrual); err != nil {
        return err
    }

    if statusKey == StatusProcessed {

    	txStmt = tx.StmtContext(ctx, updateBalance)
	    if _, err = txStmt.ExecContext(ctx, newBalance, withdrawn, userToken); err != nil {
	        return err
	    }
    }

    
    return tx.Commit()

}
