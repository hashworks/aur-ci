package main

import (
	"flag"
	"log"
	"os"

	"github.com/hetznercloud/hcloud-go/hcloud"
	_ "github.com/mattn/go-sqlite3"
	"xorm.io/xorm"

	_ "github.com/hashworks/aur-ci/controller/docs"
	"github.com/hashworks/aur-ci/controller/model"
	"github.com/hashworks/aur-ci/controller/server"
	"github.com/robfig/cron/v3"
)

// @title AUR CI Controller
// @version 1.0
// @description Continuous Integration Controller for the Arch Linux User Repository
// @contact.name Justin Kromlinger
// @contact.url https://hashworks.net
// @contact.email justin.kromlinger@stud.htwk-leipzig.de
// @license.name GNU General Public License v3
// @license.url https://www.gnu.org/licenses/gpl-3.0
// @BasePath /api
func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "Address to bind")
	driver := flag.String("driver", "sqlite3", "Database driver")
	dsn := flag.String("dsn", "file::memory:?cache=shared", "Database data source name")
	gitStoragePath := flag.String("git", "./git", "Git storage path")
	hetznerToken := flag.String("hetzner", os.Getenv("HETZNER_API_TOKEN"), "Hetzner API Token [$HETZNER_API_TOKEN]")
	hetznerSSHKeyName := flag.String("hetznerSSHKey", "", "Hetzner SSH Key Name")
	flag.Parse()

	if len(*driver) == 0 {
		log.Fatal("Missing database driver")
	}
	if len(*dsn) == 0 {
		log.Fatal("Missing database data source name")
	}
	if len(*hetznerToken) == 0 {
		log.Fatal("Missing hetzner API token")
	}

	server := server.Server{
		GitStoragePath:    gitStoragePath,
		HetznerSSHKeyName: hetznerSSHKeyName,
		HetznerClient:     hcloud.NewClient(hcloud.WithToken(*hetznerToken)),
		DB:                createDatabaseEngine(driver, dsn),
	}

	initializeDatabase(server.DB)
	defer server.DB.Close()

	c := cron.New()
	//c.AddFunc("@every 5m", server.CheckVMStatus)
	c.Start()
	server.CheckVMStatus()

	routerEngine := server.NewRouter()

	log.Printf("Starting AUR CI Controller on %s\n", *addr)

	log.Fatal(routerEngine.Run(*addr))
}

func initializeDatabase(engine *xorm.Engine) {
	err := engine.Sync2(new(model.Package), new(model.Commit), new(model.Worker), new(model.Build))
	if err != nil {
		log.Fatal("Failed to sync structs to database tables: " + err.Error())
	}
}

func createDatabaseEngine(driver *string, dsn *string) *xorm.Engine {
	engine, err := xorm.NewEngine(*driver, *dsn)
	if err != nil {
		log.Fatal("Failed to open database connection: " + err.Error())
	}

	return engine
}
