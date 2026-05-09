package engine

import (
	"context"
	"sync"
)

type documentValidation struct {
	index    int
	issues   []Issue
	warnings []Warning
	err      error
	skipped  bool
	message  string
}

type parsedDocument struct {
	file     DiscoveredFile
	document *Document
	err      error
}

type schemaStoreCatalogLoad struct {
	catalog *schemaStoreCatalog
	warning *Warning
	err     error
}

func loadSchemaStoreCatalogAsync(ctx context.Context, cache *SchemaCache, cfg Config) <-chan schemaStoreCatalogLoad {
	results := make(chan schemaStoreCatalogLoad, 1)
	go func() {
		catalog, warning, err := loadSchemaStoreCatalog(ctx, cache, cfg)
		results <- schemaStoreCatalogLoad{catalog: catalog, warning: warning, err: err}
	}()
	return results
}

func parseDocuments(files []DiscoveredFile, cfg Config, sourceLocations bool) []parsedDocument {
	results := make([]parsedDocument, len(files))
	if len(files) == 0 {
		return results
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workerCount(cfg.Schemas.Concurrency, len(files)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				document, err := parseDocument(files[index], cfg.Parsing, sourceLocations)
				results[index] = parsedDocument{
					file:     files[index],
					document: document,
					err:      err,
				}
			}
		}()
	}
	for index := range files {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

func validateDocuments(ctx context.Context, cache *SchemaCache, cfg Config, documents []*Document) []documentValidation {
	results := make([]documentValidation, len(documents))
	if len(documents) == 0 {
		return results
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workerCount(cfg.Schemas.Concurrency, len(documents)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				validation := validateDocument(ctx, cache, cfg, documents[index])
				validation.index = index
				results[index] = validation
			}
		}()
	}
	for index := range documents {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

func workerCount(configured, jobs int) int {
	if configured < 1 {
		configured = 1
	}
	if configured > jobs {
		return jobs
	}
	return configured
}
