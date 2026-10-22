package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	identityapp "github.com/lynx/lynx-os/backend/internal/application/identity"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/postgres"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL não configurada")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("não foi possível conectar ao PostgreSQL")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return errors.New("PostgreSQL indisponível")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Nome do owner: ")
	displayName, err := reader.ReadString('\n')
	if err != nil {
		return errors.New("não foi possível ler o nome")
	}
	fmt.Print("E-mail: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return errors.New("não foi possível ler o e-mail")
	}
	fmt.Print("Senha (mínimo 12 caracteres): ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return errors.New("não foi possível ler a senha")
	}
	fmt.Print("Confirme a senha: ")
	confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil || string(password) != string(confirmation) {
		return errors.New("as senhas não conferem")
	}
	if err := identityapp.CreateInitialOwner(ctx, postgres.NewIdentityRepository(pool), strings.TrimSpace(email), string(password), strings.TrimSpace(displayName)); err != nil {
		if errors.Is(err, identityapp.ErrInvalidAccount) {
			return errors.New("dados inválidos: confira nome, e-mail e senha (12 a 72 bytes)")
		}
		return err
	}
	fmt.Println("Conta owner criada. O bootstrap não pode ser repetido enquanto existir um owner.")
	return nil
}
