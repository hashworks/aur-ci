package main

import (
	"flag"
	"log"
	"os"

	"github.com/hetznercloud/hcloud-go/hcloud"
	_ "github.com/mattn/go-sqlite3"
	"xorm.io/xorm"

	"github.com/cheggaaa/pb/v3"
	"github.com/hashworks/aur-ci/controller/aur"
	_ "github.com/hashworks/aur-ci/controller/docs"
	"github.com/hashworks/aur-ci/controller/model"
	"github.com/hashworks/aur-ci/controller/server"
	"github.com/robfig/cron/v3"
)

func getEnv(key string, defaultValue string) string {
	v := os.Getenv(key)
	if len(v) == 0 {
		return defaultValue
	}
	return v
}

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
	addr := flag.String("addr", getEnv("ADDRESS", "127.0.0.1:8080"), "Address to bind")
	externalURI := flag.String("external-uri", getEnv("EXTERNAL_URI", "http://127.0.0.1:8080"), "External uri")
	driver := flag.String("driver", getEnv("DB_DRIVER", "sqlite3"), "Database driver [$DB_DRIVER]")
	dsn := flag.String("dsn", getEnv("DB_DSN", "file::memory:?cache=shared"), "Database data source name [$DB_DSN")
	gitStoragePath := flag.String("git", getEnv("GIT_STORAGE_PATH", "./git"), "Git storage path [$GIT_STORAGE_PATH]")
	hetznerToken := flag.String("hetzner", getEnv("HETZNER_API_TOKEN", ""), "Hetzner API Token [$HETZNER_API_TOKEN]")
	hetznerSSHKeyName := flag.String("hetznerSSHKey", getEnv("HETZNER_SSH_KEY", ""), "Hetzner SSH Key Name [$HETZNER_SSH_KEY]")
	initializeGit := flag.Bool("initializeGit", false, "Initialize or update git repositories")
	flag.Parse()

	if len(*addr) == 0 {
		log.Fatal("Missing address")
	}
	if len(*externalURI) == 0 {
		log.Fatal("Missing external URI")
	}
	if len(*driver) == 0 {
		log.Fatal("Missing database driver")
	}
	if len(*dsn) == 0 {
		log.Fatal("Missing database data source name")
	}
	if len(*hetznerToken) == 0 {
		log.Fatal("Missing hetzner API token")
	}

	if *initializeGit {
		initializeOrUpdateGitRepositories(gitStoragePath)
		os.Exit(0)
	}

	server := server.Server{
		GitStoragePath:    gitStoragePath,
		HetznerSSHKeyName: hetznerSSHKeyName,
		HetznerClient:     hcloud.NewClient(hcloud.WithToken(*hetznerToken)),
		ExternalURI:       externalURI,
		DB:                createDatabaseEngine(driver, dsn),
	}

	initializeDatabase(server.DB)
	defer server.DB.Close()

	c := cron.New()
	c.AddFunc("@every 5m", server.CheckVMStatus)
	c.Start()

	routerEngine := server.NewRouter()

	log.Printf("Starting AUR CI Controller on %s\n", *addr)

	log.Fatal(routerEngine.Run(*addr))
}

func initializeDatabase(engine *xorm.Engine) {
	err := engine.Sync2(new(model.Package), new(model.Commit), new(model.Worker), new(model.Build), new(model.WorkResult))
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

func initializeOrUpdateGitRepositories(gitStoragePath *string) {
	pkgBases, err := aur.GetPackageBases()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Initializing %d package bases…", len(pkgBases))
	bar := pb.StartNew(len(pkgBases))
	for _, pkgBase := range pkgBases {
		bar.Increment()

		repo, err := aur.CloneOrFetchRepository(gitStoragePath, pkgBase)
		if err != nil {
			log.Printf("Failed to clone/fetch %s: %s", pkgBase, err)
			continue
		}
		ref, err := repo.Head()
		if err != nil {
			log.Printf("Failed to get head of %s: %s", pkgBase, err)
			continue
		}
		_, err = aur.GetCommitTAR(gitStoragePath, pkgBase, ref.Hash().String())
		if err != nil {
			log.Printf("Failed to create head TAR of %s: %s", pkgBase, err)
			continue
		}
	}

	bar.Finish()
}
