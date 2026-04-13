package app

import (
	"encoding/json"
	"fmt"
	"log"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

type queryArgs struct {
	project string
	docType string
	search  string
	limit   int
}

func parseQueryArgs(args []string) queryArgs {
	qa := queryArgs{limit: 10}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if i+1 < len(args) {
				qa.project = args[i+1]
				i++
			}
		case "--type":
			if i+1 < len(args) {
				qa.docType = args[i+1]
				i++
			}
		case "--search":
			if i+1 < len(args) {
				qa.search = args[i+1]
				i++
			}
		case "--limit":
			if i+1 < len(args) {
				if n, err := fmt.Sscanf(args[i+1], "%d", &qa.limit); err != nil || n != 1 {
					qa.limit = 10
				}
				i++
			}
		}
	}
	return qa
}

func runQuery(args []string) {
	qa := parseQueryArgs(args)

	db, err := store.InitDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	var summaries []store.DocumentSummary

	if qa.search != "" {
		summaries, err = store.KeywordSearch(db, qa.search, qa.project, qa.limit)
		if err != nil {
			log.Fatalf("search failed: %v", err)
		}
	} else {
		summaries, err = store.QueryDocuments(db, qa.docType, qa.project, "", "", nil, qa.limit)
		if err != nil {
			log.Fatalf("query failed: %v", err)
		}
	}

	data, err := json.Marshal(summaries)
	if err != nil {
		log.Fatalf("marshal failed: %v", err)
	}

	fmt.Println(string(data))
}

type itemsArgs struct {
	project string
	status  string
	limit   int
}

func parseItemsArgs(args []string) itemsArgs {
	ia := itemsArgs{status: "open", limit: 20}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if i+1 < len(args) {
				ia.project = args[i+1]
				i++
			}
		case "--status":
			if i+1 < len(args) {
				ia.status = args[i+1]
				i++
			}
		case "--limit":
			if i+1 < len(args) {
				if n, err := fmt.Sscanf(args[i+1], "%d", &ia.limit); err != nil || n != 1 {
					ia.limit = 20
				}
				i++
			}
		}
	}
	return ia
}

func runItems(args []string) {
	ia := parseItemsArgs(args)

	db, err := store.InitDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	project, err := backlog.GetProjectByName(db, ia.project)
	if err != nil {
		fmt.Println("[]")
		return
	}

	filter := backlog.ItemFilter{ProjectID: &project.ID}
	if ia.status != "" {
		filter.Status = &ia.status
	}

	items, err := backlog.ListItems(db, filter)
	if err != nil {
		log.Fatalf("list items failed: %v", err)
	}

	data, err := json.Marshal(items)
	if err != nil {
		log.Fatalf("marshal failed: %v", err)
	}

	fmt.Println(string(data))
}
