package main

import (
	"fmt"
	"log"

	"embyforge/internal/model"
)

func main() {
	db, err := model.InitDB("data/embyforge.db")
	if err != nil {
		log.Fatal(err)
	}

	// 查询 WebhookConfig
	var webhookConfigs []model.WebhookConfig
	db.Find(&webhookConfigs)
	
	fmt.Println("=== Webhook Configs ===")
	for _, config := range webhookConfigs {
		fmt.Printf("ID: %d\n", config.ID)
		fmt.Printf("Repo URL: %s\n", config.RepoUrl)
		fmt.Printf("Branch: %s\n", config.Branch)
		fmt.Printf("File Path: %s\n", config.FilePath)
		fmt.Printf("Webhook URL: %s\n", config.WebhookUrl)
		fmt.Printf("Secret: %s (encrypted)\n", config.Secret)
		fmt.Printf("Created At: %s\n", config.CreatedAt)
		fmt.Println("---")
	}

	// 查询 WebhookLog
	var webhookLogs []model.WebhookLog
	db.Find(&webhookLogs)
	
	fmt.Println("\n=== Webhook Logs ===")
	for _, log := range webhookLogs {
		fmt.Printf("ID: %d\n", log.ID)
		fmt.Printf("Source: %s\n", log.Source)
		fmt.Printf("Repo Name: %s\n", log.RepoName)
		fmt.Printf("Branch: %s\n", log.Branch)
		fmt.Printf("Commit SHA: %s\n", log.CommitSHA)
		fmt.Printf("Success: %v\n", log.Success)
		fmt.Printf("Error Msg: %s\n", log.ErrorMsg)
		fmt.Printf("Created At: %s\n", log.CreatedAt)
		fmt.Println("---")
	}

	// 查询 SystemConfig 中的 Symedia 配置
	var symediaUrl model.SystemConfig
	db.Where("key = ?", "symedia_url").First(&symediaUrl)
	
	var symediaToken model.SystemConfig
	db.Where("key = ?", "symedia_auth_token").First(&symediaToken)
	
	fmt.Println("\n=== System Config (Symedia) ===")
	fmt.Printf("Symedia URL: %s\n", symediaUrl.Value)
	fmt.Printf("Symedia Auth Token: %s (encrypted)\n", symediaToken.Value)
}
