package migration

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"code.google.com/p/google-api-go-client/compute/v1"
)

var MigrationStartHandler = "/migration/start"
var MigrationMigrateHandler = "/migration/migrate"

var hostname = ""

func init() {
	name, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	hostname = name
}

func RegisterHandlers(gceService *compute.Service) {
	http.HandleFunc(MigrationStartHandler, func(w http.ResponseWriter, r *http.Request) {
		err := handleMigrationStart(w, r, gceService)
		if err != nil {
			fmt.Fprintf(w, "%s", err)
		}
	})

	http.HandleFunc(MigrationMigrateHandler, func(w http.ResponseWriter, r *http.Request) {
		request := MigrationRequest{
			Container: "c7d4f0543e92",
			Host:      "vertigo-0",
			Port:      8080,
			Command:   []string{"/bin/sleep", "2m"},
		}
		err := handleMigration(request, "vertigo-1:8080", gceService)
		if err != nil {
			fmt.Fprintf(w, "%s", err)
		}
	})
}
