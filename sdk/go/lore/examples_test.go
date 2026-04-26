package lore_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	lore "obsidian-harness/sdk/go/lore"
)

func ExampleStart() {
	ctx := context.Background()
	client, err := lore.Start(ctx, lore.Options{
		Command:   "lore",
		WorkDir:   "./tmp/chat-demo",
		ClientKey: os.Getenv("LORE_CLIENT_KEY"),
	})
	if err != nil {
		log.Print(err)
		return
	}
	defer client.Close()

	resolved, err := client.VaultResolve(ctx, lore.VaultResolveRequest{
		Query: "persona",
		Limit: 5,
	})
	if err != nil {
		log.Print(err)
		return
	}
	fmt.Println(resolved.Status)
}

func ExampleToolError() {
	var err error
	var toolErr *lore.ToolError
	if errors.As(err, &toolErr) {
		log.Printf("tool %s failed: %s", toolErr.Tool, toolErr.Message)
	}
}
