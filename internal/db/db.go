package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/denisenkom/go-mssqldb"
)

// Connect abre e valida a conexão com o SQL Server via Trusted Connection.
// Registra o erro e encerra o processo com os.Exit(1) em caso de falha.
func Connect() *sql.DB {
	dbServer := os.Getenv("DB_SERVER")
	if dbServer == "" {
		dbServer = `DESKTOP-B1QQIIN\SQLEXPRESS`
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "srcoff"
	}

	connStr := fmt.Sprintf(
		"server=%s;database=%s;trusted_connection=yes",
		dbServer, dbName,
	)

	db, err := sql.Open("mssql", connStr)
	if err != nil {
		log.Printf("erro ao abrir conexão com o banco de dados: %v", err)
		os.Exit(1)
	}

	if err = db.Ping(); err != nil {
		log.Printf("erro ao verificar conexão com o banco de dados: %v", err)
		os.Exit(1)
	}

	return db
}
