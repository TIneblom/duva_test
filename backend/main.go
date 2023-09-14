package main

import (
  "context"
  "fmt"
  "net/http"
  neturl "net/url"
  "math/rand"
  "strings"
  "github.com/redis/go-redis/v9"
)

// 6st av 62 olika tecken blir max 62^6 ≈ 56'800'235'584 unika länkar 
const SHORT_LENGTH = 6
const SHORT_LETTERS = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" 

func processLongUrl(longUrl string) (string, error) {
  if len(longUrl) <= 3 {
    return "", fmt.Errorf("Too short! 3+ characters required.");
  }

  result := longUrl

  scheme_end_index := strings.Index(longUrl, ":")
  if scheme_end_index == -1 {
    result = "http://" + longUrl;
  }

  url, err := neturl.Parse(result)
  if err != nil {
    return "", err
  }

  if len(url.Scheme) == 0 || len(url.Host) == 0 {
    return "", fmt.Errorf("Scheme and host required: %v", longUrl)
  }

  last_dot_index := strings.LastIndex(url.Host, ".")
  if last_dot_index == -1 { 
    return "", fmt.Errorf("Host does not contain a dot -> '%v'", result)
  }

  return result, nil
}

func shortenURL(ctx context.Context, url string, rdb *redis.Client) (string, error) {
  result := make([]byte, SHORT_LENGTH) 

  for {
    for i := 0; i < SHORT_LENGTH; i++ {
      result[i] = SHORT_LETTERS[rand.Intn(len(SHORT_LETTERS))];
    }

    exists, err := rdb.Exists(ctx, string(result)).Result()
    if err != nil {
      return "", err
    }
    if exists == 0 {
      break
    }
  }
  
  return string(result), nil
}

func getLongFromShortURL(ctx context.Context, rdb *redis.Client, shortUrl string) (string, error) {
  if len(shortUrl) == SHORT_LENGTH {
    for i := 0; i < SHORT_LENGTH; i++ {
      if (shortUrl[i] >= 48 && shortUrl[i] <= 57) ||
         (shortUrl[i] >= 65 && shortUrl[i] <= 90) ||
         (shortUrl[i] >= 97 && shortUrl[i] <= 122) {
        continue
      }

      return "", fmt.Errorf("URL contains invalid characters")
    }

    longURL, err := rdb.Get(ctx, shortUrl).Result()
    if err != nil {
      return "", fmt.Errorf("Could not find short url")
    }

    return longURL, nil
  }

  return "", fmt.Errorf("Invalid URL")
}


func main() {
  ctx := context.Background()

  rdb := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
    Password: "",
    DB: 0,
  })

  sessions := make(map[string]Session)

  requestHandler := RequestHandler{
    ctx: ctx,
    rdb: rdb,
    sessions: sessions,
  }

  http.HandleFunc("/", requestHandler.handleHome)
  http.HandleFunc("/api/shorten", requestHandler.handleShorten)
  http.HandleFunc("/api/getSessionUsername", requestHandler.getSessionUsername)
  http.HandleFunc("/api/login", requestHandler.handleLogin)
  http.HandleFunc("/api/logout", requestHandler.handleLogout)
  http.HandleFunc("/api/register", requestHandler.handleRegister)
  http.HandleFunc("/api/getLinks", requestHandler.getLinks)
  http.HandleFunc("/api/getLinkData", requestHandler.getLinkData)
  http.HandleFunc("/api/removeLink", requestHandler.removeLink)

  fmt.Println("Listening on http://localhost:8080")
  http.ListenAndServe(":8080", nil)
}
