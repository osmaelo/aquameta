package main

import (
    "context"
    "flag"
    "fmt"
    embeddedPostgres "github.com/aquametalabs/embedded-postgres"
    "github.com/jackc/pgx/v4/pgxpool"
    "github.com/lib/pq"
    "log"
    "net/http"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "syscall"
    "time"
)

func main() {
    log.Print(`                                           __          `)
    log.Print(`_____    ________ _______    _____   _____/  |______   `)
    log.Print(`\__  \  / ____/  |  \__  \  /     \_/ __ \   __\__  \  `)
    log.Print(` / __ \< <_|  |  |  // __ \|  Y Y  \  ___/|  |  / __ \_`)
    log.Print(`(____  /\__   |____/(____  /__|_|  /\___  >__| (____  /`)
    log.Print(`     \/    |__|          \/      \/     \/          \/ `)
    log.Print(`                 [ version 0.5.0 ]                     `)

    // log.SetPrefix("[💧 aquameta 💧] ")
    log.Print("Aquameta server... ENGAGE!")
    workingDirectory, err := filepath.Abs(filepath.Dir(os.Args[0]))
    var epg embeddedPostgres.EmbeddedPostgres

    //
    // trap ctrl-c
    //
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        for sig := range c {
            if epg.IsStarted() {
                log.Print("Stopping PostgreSQL")
                epg.Stop()

            }
            log.Fatalf("SIG %s - Good day.", sig)
        }
    }()

    //
    // load config
    //
    var configFile = flag.String("c", "", "configuration file")
    flag.Parse()

    config, err := getConfig(*configFile)
    if err != nil {
        log.Printf("Could not load boot configuration file: %s", err)
        log.Print("Usage:")
        flag.PrintDefaults()
        log.Fatal("Quitting.")
        /*
           log.Printf("Loading default Bootloader configuration instead from %s", bootloaderConfigFile)

           blconfig, err := getConfig(bootloaderConfigFile); if err != nil {
               log.Fatalf("Could not load bootloader config %s: %s", bootloaderConfigFile, err)
           }
           config = blconfig
        */
    }

    //
    // setup embedded database
    //
    if config.Database.Mode == "embedded" {
        //
        // initialize epg w/ config settings
        //

        // TODO: NewDatabase() should be called NewPGServer() or some such... refactor epg
        epg = *embeddedPostgres.NewDatabase(embeddedPostgres.DefaultConfig().
            Username(config.Database.Role).
            Password(config.Database.Password).
            // Host
            Port(config.Database.Port).
            Database(config.Database.DatabaseName).
            Version(embeddedPostgres.V12).
            RuntimePath(config.Database.EmbeddedPostgresRuntimePath).
            StartTimeout(45 * time.Second))

        // has an embedded postgres already been installed?
        log.Printf("Checking for existing embedded server at %s", config.Database.EmbeddedPostgresRuntimePath)
        epgFilesExist := true
        if _, err := os.Stat(config.Database.EmbeddedPostgresRuntimePath); os.IsNotExist(err) {
            // TODO: we probably want some more robust inspection of the directory.
            // Check that it has the binary, and a data directory, and generally looks sane.
            // If it doesn't, QUIT!  (Do NOT install the db here, it might be some other directory
            // that would get overwritten.
            log.Printf("Embedded PostgreSQL server found at %s.", config.Database.EmbeddedPostgresRuntimePath)
            epgFilesExist = false
        }

        // if directory doesn't exist, generate an embedded database there
        if !epgFilesExist {
            log.Printf("Embedded PostgreSQL server not found at %s.  Installing...", config.Database.EmbeddedPostgresRuntimePath)

            if err := epg.Install(); err != nil {
                log.Fatalf("Unable to install PostgreSQL: %v", err)
            }
            log.Printf("PostgreSQL server installed at %s", config.Database.EmbeddedPostgresRuntimePath)
        }

        //
        // start the epg database daemon
        //
        log.Printf("Starting PostgreSQL server from %s...", config.Database.EmbeddedPostgresRuntimePath)
        if err := epg.Start(); err != nil {
            log.Fatalf("Unable to start PostgreSQL: %v", err)
        }
        log.Print("PostgreSQL server started.")

        defer func() {
            log.Print("Stopping PostgreSQL Server...")
            if err := epg.Stop(); err != nil {
                log.Fatalf("Database halt failed: %v", err)
            } else {
                log.Print("Database stopped")
            }
        }()

        //
        // CREATE DATABASE
        //
        if !epgFilesExist {
            if err := epg.CreateDatabase(); err != nil {
                // TODO: create epg.DatabaseExists() method
                // log.Fatalf("Unable to create database: %v", err)
            } else {
                log.Print("PostgreSQL server installed to %s", config.Database.EmbeddedPostgresRuntimePath)
            }
        }
    }

    //
    // connect to database
    //
    connectionString := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", config.Database.Role, config.Database.Password, config.Database.Host, config.Database.Port, config.Database.DatabaseName)
    log.Printf("Database: %s", connectionString)

    dbpool, err := pgxpool.Connect(context.Background(), connectionString)
    if err != nil {
        log.Fatalf("Unable to connect to database: %v", err)
    }
    log.Print("Connected to database.")
    defer dbpool.Close()

    //
    // pg_settings
    //
    // CLI switch here:  if --debug, else ...
    settingsQueries := [...]string{
        fmt.Sprintf("set log_min_messages='notice'"), // { notice, warning, error, ...}
        fmt.Sprintf("set log_statement='all'"),
        fmt.Sprintf("set statement_timeout=0"),
    }
    for i := 0; i < len(settingsQueries); i++ {
        _, err := dbpool.Exec(context.Background(), settingsQueries[i])
        if err != nil {
            epg.Stop()
            log.Fatalf("Unable to update settings: %v", err)
        }
    }
    log.Print("PostgreSQL settings have been set.")

    //
    // - install aquameta extensions
    //
    var ct int
    dbQuery := fmt.Sprintf("select count(*) as ct from pg_catalog.pg_extension where extname in ('meta','meta_triggers','pg_bundle','event','endpoint','ide','documentation','widget','semantics')")
    err = dbpool.QueryRow(context.Background(), dbQuery).Scan(&ct)
    log.Print("Checking for Aquameta installation....")

    // TODO: handle this with a flag instead. Stop installing by default.
    if ct != 9 {

        //
        // install aquameta extensions
        //
        log.Print("Aquameta is not installed on this database.  Installing...")

        if config.Database.Mode == "embedded" {
            exec.Command("/bin/sh", "-c", "cp "+workingDirectory+"/extensions/*/*--*.*.*.sql "+config.Database.EmbeddedPostgresRuntimePath+"/share/postgresql/extension/").Run()
            exec.Command("/bin/sh", "-c", "cp "+workingDirectory+"/extensions/*/*.control "+config.Database.EmbeddedPostgresRuntimePath+"/share/postgresql/extension/").Run()
            log.Print("Extensions copied to PostgreSQL's extensions directory.")
        }

        installQueries := [...]string{
            "create extension if not exists hstore schema public",
            "create extension if not exists \"uuid-ossp\" schema public",
            // "create extension if not exists pg_uuidv7 schema public",
            "create extension if not exists pgcrypto schema public",
            "create extension if not exists postgres_fdw schema public",
            "create extension meta version '0.5.0'",
            "create extension meta_triggers version '0.5.0'",
            "create extension pg_bundle version '0.5.0'",
            "create extension event version '0.5.0'",
            "create extension endpoint version '0.5.0'",
            "create extension widget version '0.5.0'",
            "create extension semantics version '0.5.0'",
            "create extension ide version '0.5.0'",
            "create extension documentation version '0.5.0'"}

        for i := 0; i < len(installQueries); i++ {
            log.Print(installQueries[i])
            _, err := dbpool.Exec(context.Background(), installQueries[i])
            if err != nil {
                log.Fatalf("Unable to install extensions: %v", err)
                if config.Database.Mode == "embedded" {
                    epg.Stop()
                }
            }
        }
        log.Print("Extensions were successfully installed.")

        //
        // setup hub remote
        //
        /*
        log.Print("Adding bundle.remote_database for hub...")
        hubRemoteQuery := `insert into bundle.remote_database (foreign_server_name, schema_name, connection_string, username, password)
            values (
                'hub', 'hub',
                'dbname ''aquameta'', host ''hub.aquameta.com'', port ''5432''',
                'anonymous', 'anonymous'
            )`
        _, err := dbpool.Query(context.Background(), hubRemoteQuery)
        if err != nil {
            if config.Database.Mode == "embedded" {
                epg.Stop()
            }
            log.Fatalf("Unable to add bundle.remote_database: %v", err)
        }
        */

        //
        // create superuser
        //
        log.Print("Setting up permissions...")

        superuserQuery := fmt.Sprintf("insert into endpoint.user (email, name, active, role_id) values (%s, %s, true, meta.role_id(%s))",
            pq.QuoteLiteral(config.AquametaUser.Email),
            pq.QuoteLiteral(config.AquametaUser.Name),
            pq.QuoteLiteral(config.Database.Role))
        rows, err := dbpool.Query(context.Background(), superuserQuery)
        if err != nil {
            log.Fatalf("Unable to create superuser: %v", err)
        }
        rows.Close()

        //
        // download and install bundles
        //
        /*
           TODO: switch hub install vs local file install, based on CLI

           // hub install over network
           log.Print("Downloading Aquameta core bundles from hub.aquameta.com...")
           bundleQueries := [...]string{
               "select bundle.remote_mount(id) from bundle.remote_database",
               "select bundle.remote_pull_bundle(r.id, b.id) from bundle.remote_database r, hub.bundle b",
               "select bundle.checkout(c.id) from bundle.commit c join bundle.bundle b on b.head_commit_id = c.id;" }

           for i := 0; i < len(bundleQueries); i++ {
               log.Printf("Setup query: %s", bundleQueries[i])
               rows, err := dbpool.Query(context.Background(), bundleQueries[i])
               if err != nil {
                   log.Fatalf("Unable to install Aquameta bundles: %v", err)
               }
               rows.Close()
           }
        */

        // install from local filesystem
        log.Print("Installing core bundles from source")
        coreBundles := [...]string{
            "org.aquameta.core.mimetypes",
            "org.aquameta.core.endpoint",
            "org.aquameta.core.widget",
            "org.aquameta.core.ide",
            "org.aquameta.core.semantics",
            "org.aquameta.games.snake",
            "org.aquameta.ui.fsm",
            "org.aquameta.ui.layout",
            "org.aquameta.ui.tags",
            "org.aquameta.core.bootloader",
            // "org.aquameta.core.repository",
        }

        for i := 0; i < len(coreBundles); i++ {
            log.Print("  - "+coreBundles[i])
            q := "select bundle.import_repository(pg_read_file('" + workingDirectory + "/bundles/" + coreBundles[i] + ".json'))"
            _, err := dbpool.Exec(context.Background(), q)
            if err != nil {
                if config.Database.Mode == "embedded" {
                    epg.Stop()
                }
                log.Fatalf("Unable to install Aquameta bundles: %v", err)
            }

            _, err = dbpool.Exec(context.Background(), "select bundle.checkout('" + coreBundles[i] + "')")
            if err != nil {
                log.Fatalf("Unable to checkout core bundles: %v", err)
            }
        }

        log.Print("Installation complete!")

    }

    bootloaderHandler := func(w http.ResponseWriter, req *http.Request) {

        log.Println(req.Proto, req.Method, req.RequestURI)

        // halt
        if req.RequestURI == "/bootloader/halt" {
            log.Print("Bootloader has requested that I halt, so I will halt.")
            if epg.IsStarted() {
                log.Print("Stopping PostgreSQL")
                epg.Stop()
            }

            log.Fatal("Good day.")
        }

        // write config
        if req.RequestURI == "/bootloader/configure" {
            log.Println("Ok I will write out the specified .conf file to disk")
        }
    }

    //
    // attach handlers
    //
    // TODO: configure these in the database??
    http.HandleFunc("/_socket/detach/", websocketDetach)
    http.Handle("/socket.io/", websocket(dbpool))
    http.HandleFunc("/bootloader/", bootloaderHandler)
    http.HandleFunc("/endpoint/", endpoint(dbpool))
    http.HandleFunc("/", resource(dbpool))

    httpDone := make(chan bool)
    fuseDone := make(chan bool)

    //
    // start http server
    //
    log.Printf("Starting HTTP server\n\n%s://%s:%s%s\n\n",
        config.HTTPServer.Protocol,
        config.HTTPServer.IP,
        config.HTTPServer.Port,
        config.HTTPServer.StartupURL)

    go func() {
        if config.HTTPServer.Protocol == "http" {
            http.ListenAndServe(config.HTTPServer.IP+":"+config.HTTPServer.Port, nil)
        } else {
            if config.HTTPServer.Protocol == "https" {
                // https://github.com/denji/golang-tls
                log.Fatal(http.ListenAndServeTLS(
                    config.HTTPServer.IP+":"+config.HTTPServer.Port,
                    config.HTTPServer.SSLCertificateFile,
                    config.HTTPServer.SSLKeyFile,
                    nil))
            } else {
                log.Fatal("Unrecognized protocol: " + config.HTTPServer.Protocol)
            }
        }

        httpDone <- true

    }()


    //
    // start pgfs (if OS is supported and it's enabled in config)
    //

    go pgfs(config, dbpool, fuseDone)


    //
    // start gui
    //
    /*
       log.Printf("HTTP server started, startup URL:\n\n%s://%s:%s%s\n\n",
           config.HTTPServer.Protocol,
           config.HTTPServer.IP,
           config.HTTPServer.Port,
           config.HTTPServer.StartupURL)

       w := webview.New(true)
       defer w.Destroy()
       w.SetTitle("Aquameta Boot Loader")
       w.SetSize(800, 500, webview.HintNone)
       w.Navigate(config.HTTPServer.Protocol+"://"+config.HTTPServer.IP+":"+config.HTTPServer.Port+"/boot")
       w.Run()
    */

    select {
    case <-httpDone:
        println("HTTP server stopped.")
    case <-fuseDone:
        println("FUSE filesystem stopped.")
    }

    if config.Database.Mode == "embedded" {
        if epg.IsStarted() {
            epg.Stop()
        }
    }

    log.Fatal("Good day.")
}
