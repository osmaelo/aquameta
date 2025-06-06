package main

import (
    "context"
    "encoding/json"
    "fmt"
    "github.com/jackc/pgx/v4/pgxpool"
    "github.com/lib/pq"
    "io"
    "log"
    "net/http"
    "net/url"
)

func resource(dbpool *pgxpool.Pool) func(w http.ResponseWriter, req *http.Request) {
    /*
     * resource handler
     *
     * 1. count the number of matching paths in
     *   - endpoint.resource
     *   - endpoint.resource_binary
     *   - endpoint.template_route
     * if count > 1, throw a 300 multiple choices
     * if count < 1, throw 404 not found
     *
     * 2. grab the resource or template or function, serve the content
     */
    resourceHandler := func(w http.ResponseWriter, req *http.Request) {
        log.Println(req.Proto, req.Method, req.RequestURI)

        // path
        // path := strings.SplitN(req.RequestURI,"?", 2)[0]
        path, err := url.QueryUnescape(req.URL.Path)
        if err != nil {
            log.Fatal(err)
        }

        // query string
        /*
           m, err := url.ParseQuery(req.URL.RawQuery)
           if err != nil { log.Fatal(err) }
        */

        // count matching endpoint.resource
        // TODO: Learn to work with UUIDs in Go
        const matchCountQ = `
            select r.id::text, 'resource' as resource_table
            from endpoint.resource r
            where path = %v
            and active = true

            union

            select r.id::text, 'resource_binary'
            from endpoint.resource_binary r
            where path = %v
            and active = true

            union

            select r.id::text, 'resource_function'
            from endpoint.resource_function r
            -- 1. rewrite path_pattern to a regex:
            --     /blog/{$1}/article/{$2} goes to ^/blog/([^\/\s]+)/article/([^\/\s]+)$
            -- 2. match against the request path
            where %v ~ regexp_replace('^' || r.path_pattern || '$', '\${\d+}', '([^\/\s]+)', 'g')`

        /*
           union

           select r.id::text, 'template'
           from endpoint.template_route r
           where %v ~ r.url_pattern`
           // and active = true ?
        */

        matches, err := dbpool.Query(context.Background(), fmt.Sprintf(
            matchCountQ,
            pq.QuoteLiteral(path),
            pq.QuoteLiteral(path),
            pq.QuoteLiteral(path)))

        if err != nil {
			log.Printf("Resource matching query failed: %v", err)
			return
        }
        defer matches.Close()


        var id string
        var resourceTable string

        var n int32
        for matches.Next() {
            err = matches.Scan(&id, &resourceTable)
            if err != nil {
                return // FIXME
            }
            n++
        }

        // 300 Multiple Choices
        if n > 1 {
            http.Error(w, http.StatusText(http.StatusMultipleChoices), http.StatusMultipleChoices)
            return // FIXME?
        } else {

        // 404 Not Found
            if n < 1 {
                http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
                return // FIXME?
            }
        }

        // 200 OK
        // get the resource/resource_binary/resource_function, process it and return the results
        var content string
        var contentBinary []byte
        var mimetype string
        var headers []byte

        switch resourceTable {
        case "resource":
            const resourceQ = `
                select r.content, m.mimetype
                from endpoint.resource r
                    join endpoint.mimetype m on r.mimetype_id = m.id
                where r.id = %v`

            err := dbpool.QueryRow(context.Background(), fmt.Sprintf(resourceQ, pq.QuoteLiteral(id))).Scan(&content, &mimetype)
            if err != nil {
                log.Printf("QueryRow failed: %v", err)
            }
            w.Header().Set("Content-Type", mimetype)
            w.WriteHeader(200)
            io.WriteString(w, content)

        case "resource_binary":
            const resourceBinaryQ = `
               select r.content, m.mimetype
               from endpoint.resource_binary r
                   join endpoint.mimetype m on r.mimetype_id = m.id
               where r.id = %v`

            err := dbpool.QueryRow(context.Background(), fmt.Sprintf(resourceBinaryQ, pq.QuoteLiteral(id))).Scan(&contentBinary, &mimetype)
            if err != nil {
                log.Printf("QueryRow failed: %v", err)
            }
            w.Header().Set("Content-Type", mimetype)
            w.WriteHeader(200)
            w.Write(contentBinary)

        case "resource_function":
            // get the endpoint.resource_function row, propagate path_pattern, defalt_args and mimetype
            const resourceFunctionPrepQ = `
                select
                    rf.path_pattern,
                    (rf.function_id).schema_name as schema_name,
                    (rf.function_id).name as function_name,
                    (rf.function_id).parameters as function_parameters,
                    rf.default_args as default_args,
                    m.mimetype,
                    regexp_match(%v, regexp_replace('^' || rf.path_pattern || '$', '\${\d+}', '([^\/\s]+)', 'g')) as args,
                    (select array_agg(m[1]::integer) from regexp_matches(rf.path_pattern, '\${(\d+)}', 'g') m),
                    mf.return_type = 'record' as returns_record
                from endpoint.resource_function rf
                    join endpoint.mimetype m on rf.mimetype_id = m.id
                    join meta.function mf on mf.id=rf.function_id
                where rf.id = %v`

            var function_parameters []string
            var default_args []string
            var path_pattern string
            var schema_name string
            var function_name string
            var path_args []string
            var path_arg_positions []int
            var returns_record bool

            err := dbpool.QueryRow(context.Background(),
                fmt.Sprintf(resourceFunctionPrepQ, pq.QuoteLiteral(path), pq.QuoteLiteral(id))).Scan(&path_pattern, &schema_name, &function_name, &function_parameters, &default_args, &mimetype, &path_args, &path_arg_positions, &returns_record)
            if err != nil {
                log.Printf("QueryRow failed: %v", err)
            }

            // args is the array of strings to be cast to their appropriate type and passed to the function
            var args = make([]string, len(function_parameters))

            // write default_args into args
            copy(args, default_args);

            for i :=0; i<len(path_arg_positions);i++ {
                // path_arg_positions, first position is 1, hence -1 for array index
                args[path_arg_positions[i]-1] = path_args[i]
            }

            // build the function's argument string
            var function_call_str = pq.QuoteIdentifier(schema_name)+"."+pq.QuoteIdentifier(function_name)+"("
            for i := 0; i<len(function_parameters);i++ {
                // not using pq.QuoteIdentifier for function_parametrs[i] here because e.g. integer is an alias for int4, but if you quote it, it uses only and exactly the literal type name.  FIXME?
                function_call_str += pq.QuoteLiteral(args[i]) + "::" + function_parameters[i];
                if i < len(function_parameters) -1 {
                    function_call_str += ","
                }
            }
            function_call_str += ")"

            var resourceFunctionQ string
            if (returns_record) {
                resourceFunctionQ = fmt.Sprintf("select content, headers from %v as (content text, headers jsonb)", function_call_str);
                err = dbpool.QueryRow(context.Background(), resourceFunctionQ).Scan(&content, &headers)
            } else {
                resourceFunctionQ = fmt.Sprintf("select %v as content", function_call_str);
                err = dbpool.QueryRow(context.Background(), resourceFunctionQ).Scan(&content)
            }

            if err != nil {
              // send 404
              w.Header().Set("Content-Type", mimetype)
              w.WriteHeader(404)
              io.WriteString(w, "")
            } else {
              // send the response
              w.Header().Set("Content-Type", mimetype)
              var extra_headers = map[string]string{}
              if len(headers) > 0 {
                err := json.Unmarshal(headers, &extra_headers)
                if err == nil {
                  for key, value := range extra_headers {
                    w.Header().Add(key, value)
                  }
                }
              }
              w.WriteHeader(200)
              io.WriteString(w, content)
            }

        /*
        failed attempt at using pl/go solution
        case "template":
            const templateQ = `
                select
                    endpoint.template_render(
                        tkid::text, -- FIXME
                        r.args::json::text, -- FIXME
                        (array_to_json( regexp_matches(%v, r.url_pattern) ))::text -- FIXME
                    ) as content,
                    m.mimetype
                from endpoint.template_route r
                    join endpoint.template t on r.template_id = t.id
                    join endpoint.mimetype m on t.mimetype_id = m.id`

            err := dbpool.QueryRow(context.Background(), fmt.Sprintf(templateQ, pq.QuoteLiteral(path))).Scan(&content, &mimetype)
            if err != nil {
                log.Printf("QueryRow failed: %v", err)
            }
            w.Header().Set("Content-Type", mimetype)
            w.WriteHeader(200)
            io.WriteString(w, content)
        */
        }
    }
    return resourceHandler
}
