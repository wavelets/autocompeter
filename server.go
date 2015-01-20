package main

import (
	"fmt"
	"github.com/codegangsta/negroni"
	"github.com/unrolled/render"
	"os"
	// "github.com/gorilla/mux"
	// "github.com/fzzy/radix/redis"
	// "github.com/xuyu/goredis"
	"github.com/garyburd/redigo/redis"
	"net/http"
	"time"
	// "log"
	"strings"
	// "strconv"
	// "regexp"
)

func CleanWords(query string) []string {
	// r := regexp.MustCompile(regexp.QuoteMeta("[]()"))
	// query = r.ReplaceAllString(query, " ")
	query = strings.Replace(query, "\"", " ", -1)
	query = strings.Replace(query, "[", " ", -1)
	query = strings.Replace(query, "]", " ", -1)
	query = strings.Replace(query, "(", " ", -1)
	query = strings.Replace(query, ")", " ", -1)
	query = strings.Replace(query, "?", " ", -1)
	query = strings.Replace(query, "!", " ", -1)
	query = strings.Replace(query, ",", " ", -1)
	query = strings.Replace(query, "-", " ", -1)
	query = strings.Replace(query, ":", " ", -1)
	query = strings.Replace(query, ";", " ", -1)
	query = strings.Replace(query, ",", " ", -1)
	query = strings.Replace(query, "'", " ", -1)
	split := strings.Fields(strings.Trim(query, " "))
	terms := make([]string, len(split))
	for i, term := range split {
		terms[i] = strings.ToLower(strings.Trim(strings.Trim(term, " "), "."))
	}
	return terms
}

// func QueryScore(terms []string, title) float32 {
// 	return 1.0
// }

func errHndlr(err error) {
	if err != nil {
		fmt.Println("error:", err)
		panic(err)
		os.Exit(1)
	}
}

func newPool(server, password string, database int) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					return nil, err
				}
			}
			_, err = c.Do("SELECT", database)
			if err != nil {
				return nil, err
			}

			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func main() {
	is_debug := os.Getenv("DEBUG")
	// fmt.Println("debug", is_debug)
	var debug = true
	if is_debug == "" {
		debug = false
	}
	renderer := render.New(render.Options{
		IndentJSON: debug,
	})

	pool := newPool("localhost:6379", "", 9)

	// config := goredis.DialConfig {
	// 	Network:  "tcp",
	// 	Address:  "127.0.0.1:6379",
	// 	Database: 9,
	// 	Password: "",
	// 	Timeout:  10*time.Second,
	// 	MaxIdle:  10,
	// }

	// c, err := goredis.DialTimeout("tcp", "127.0.0.1:6379", time.Duration(10)*time.Second)
	// c, err := goredis.Dial(&config)
	// errHndlr(err)
	// defer client.Close()

	// select database
	// r := c.Cmd("select", 9)
	// errHndlr(r.Err)

	// c, err := redis.Dial("tcp", "localhost:6379")
	// errHndlr(err)
	// defer c.Close()
	// c.Do("SELECT", 9)
	// errHndlr(err)
	// fmt.Println("database_select", database_select)

	// // Using redigo
	// c, err := redis.Dial("tcp", ":6379")
	// if err != nil {
	// 	// handle error
	// }
	// defer c.Close()
	// c.Do("SELECT", 9)
	// r, err := redis.String(c.Do("GET", "fookey"))
	// errHndlr(err)
	// fmt.Println("R", r)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Welcome to the home page!\n")
	})
	mux.HandleFunc("/v1", func(w http.ResponseWriter, req *http.Request) {
		q := strings.Trim(req.FormValue("q"), " ")
		terms := CleanWords(q)

		c := pool.Get()
		defer c.Close()

		// fmt.Println("TERMS", terms)
		// NOTE! Maybe we don't need the ZINTERSTORE if there's only 1 command
		args := make([]interface{}, len(terms)+4)
		args[0] = "$tmp"
		args[1] = len(terms)
		for i, term := range terms {
			args[i+2] = term
		}
		args[len(terms)+2] = "AGGREGATE"
		args[len(terms)+3] = "MAX"
		// fmt.Println("ARGS", args)
		c.Send("ZINTERSTORE", args...)
		c.Send("ZREVRANGE", "$tmp", 0, 10, "WITHSCORES")
		c.Flush()
		c.Receive() // the ZINTERSTORE
		// replies, err := c.Receive()
		replies, err := redis.Strings(redis.Values(c.Receive()))
		// fmt.Println("replies", replies, len(replies))
		errHndlr(err)

		ooids := make([]string, 10+1)
		scores := make([]string, 10+1)
		evens := 0
		for i, element := range replies {
			if i%2 == 0 {
				ooids[evens] = string(element)
				evens = evens + 1
			} else {
				scores[evens-1] = string(element)
			}
		}
		ooids = ooids[:evens]
		scores = scores[:evens]

		var titles []string
		if len(ooids) == 0 {
			// titles = [1]string{}
		} else {
			args := make([]interface{}, len(ooids)+1)
			args[0] = "$titles"
			for i, v := range ooids {
				args[i+1] = v
			}
			// fmt.Println("args", args)
			titles, err = redis.Strings(c.Do("HMGET", args...))
			errHndlr(err)
			// titles, err = redis.Strings(reply, err)
		}
		rows := make([]interface{}, len(titles))
		for i, title := range titles {
			row := make([]string, 2)
			row[0] = ooids[i]
			// row[1] = scores[i] * QueryScore(terms, title)
			row[1] = title
			rows[i] = row
		}
		rows = rows[:len(titles)]
		// fmt.Println("ROWS=", rows)
		// for _, title := range titles {
		// 	fmt.Println(title)
		// }

		// results := make([]interface{}, 2)
		// result := make([]string, 3)
		// result[0] = "one"
		// result[1] = "2"
		// result[2] = "three"
		// results[0] = result
		// result[0] = "four"
		// result[1] = "5"
		// result[2] = "sixe"
		// results[1] = result
		output := make(map[string]interface{})
		output["terms"] = terms
		output["results"] = rows

		w.Header().Set("Access-Control-Allow-Origin", "*")
		renderer.JSON(w, http.StatusOK, output)
	})

	// router := mux.NewRouter()
	// router.HandleFunc("/", HomeHandler)

	n := negroni.Classic()
	n.UseHandler(mux)
	// n.UseHandler(router)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}
	n.Run(":" + port)
}
