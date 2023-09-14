package main

import (
  "context"
  "fmt"
  "net/http"
  neturl "net/url"
  "io/ioutil"
  "encoding/json"
  "math/rand"
  "strings"
  "github.com/redis/go-redis/v9"
  "time"
  "strconv"
  "golang.org/x/crypto/bcrypt"
)

// 6st av 62 olika tecken blir max 62^6 ≈ 56'800'235'584 unika länkar 
const SHORT_LENGTH = 6
const SHORT_LETTERS = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" 

type LoginRequestBody struct {
  Username string
  Password string
}

type RegisterRequestBody struct {
  Username string
  Password string
}

type Session struct {
  Username string
}

type LinkData struct {
  LongURL string
  NumClicks [30]int
}

type RequestHandler struct {
  ctx context.Context
  rdb *redis.Client
  sessions map[string]Session
}

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

  // BUG: Tillåter fortfarande URL:  "http://www.hejsan"

  last_dot_index := strings.LastIndex(url.Host, ".")
  if last_dot_index == -1 { 
    return "", fmt.Errorf("Host does not contain a dot -> '%v'", result)
  }

  // TODO: Kanske http.Get för att verifiera att länkar fungerar

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

func getRequestBody[T any](r *http.Request, body *T) error {
  var bodyBytes []byte
  var err error

  if r.Body == nil {
    return fmt.Errorf("Request body nil")
  }

  defer r.Body.Close()
  bodyBytes, err = ioutil.ReadAll(r.Body)
  if err != nil {
    return err
  }

  if len(bodyBytes) == 0 {
    return fmt.Errorf("No body found")
  }

  if err = json.Unmarshal(bodyBytes, body); err != nil {
    return err
  }

  return nil
}

func (h RequestHandler) handleShorten(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  var err error

  defer r.Body.Close()
  bodyBytes, err := ioutil.ReadAll(r.Body)
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  processedUrl, err := processLongUrl(string(bodyBytes))
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  shortURL, err := shortenURL(h.ctx, processedUrl, h.rdb)
  if err != nil {
    http.Error(w, err.Error(), 400);
    return
  }

  err = h.rdb.Set(h.ctx, shortURL, processedUrl, 0).Err()
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  sessionKeyCookie, err := r.Cookie("sessionKey")
  if err == nil {
    sessionKey := sessionKeyCookie.Value
    session, found := h.sessions[sessionKey]
    if found {
      linksKey := "usr:" + session.Username + ":links"
      err = h.rdb.LPush(h.ctx, linksKey, shortURL).Err()
      if err != nil {
        http.Error(w, err.Error(), 400)
        return
      }
    }
  }

  fmt.Fprintf(w, shortURL);
}

func (h RequestHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  var body LoginRequestBody
  var err error

  err = getRequestBody(r, &body)
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  hash, err := h.rdb.Get(h.ctx, "usr:" + body.Username).Result()
  if err != nil {
    http.Error(w, "User not found: " + err.Error(), 401)
    return
  }

  err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password))
  if err == nil {
    for key, session := range h.sessions {
      if body.Username == session.Username {
        delete(h.sessions, key)
        break
      }
    } 

    const cookieChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!#$%&'*+-./:<=>?@^_`{|}~"
    sessionKey := make([]byte, 32)
    for {
      for i := 0; i < 32; i++ {
        sessionKey[i] = byte(cookieChars[rand.Intn(len(cookieChars))])
      }
      if _, found := h.sessions[string(sessionKey)]; !found {
        break
      }
    }

    h.sessions[string(sessionKey)] = Session{
      Username: body.Username,
    }

    cookie := &http.Cookie{
      Name: "sessionKey",
      Value: string(sessionKey),
      Path: "/",
      HttpOnly: true,
    }
    http.SetCookie(w, cookie)
  } else {
    http.Error(w, "Invalid password", 401)
    return
  }
}

func (h RequestHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "/")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  sessionKeyCookie, err := r.Cookie("sessionKey")
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  delete(h.sessions, sessionKeyCookie.Value)
}

func (h RequestHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "/")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  var body RegisterRequestBody
  var err error

  err = getRequestBody(r, &body)
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  usrKey := "usr:" + body.Username

  exists, err := h.rdb.Exists(h.ctx, usrKey).Result()
  if exists == 1 || err != nil {
    http.Error(w, "User already exists", 401)
    return
  } 

  hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  err = h.rdb.Set(h.ctx, usrKey, string(hash), 0).Err()
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }
}

func (h RequestHandler) getLinks(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "/")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  sessionKeyCookie, err := r.Cookie("sessionKey")
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  session, found := h.sessions[sessionKeyCookie.Value]
  if !found {
    http.Error(w, "Invalid session", 401)
    return
  }

  linksKey := "usr:" + session.Username + ":links"
  links, err := h.rdb.LRange(h.ctx, linksKey, 0, -1).Result()
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  result := strings.Join(links, " ")

  w.Header().Set("Content-Type", "text/plain")
  w.Write([]byte(result))
}

func (h RequestHandler) removeLink(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "/")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  sessionKeyCookie, err := r.Cookie("sessionKey")
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  session, found := h.sessions[sessionKeyCookie.Value]
  if !found {
    http.Error(w, "Invalid session", 401)
    return
  }

  defer r.Body.Close()
  bodyBytes, err := ioutil.ReadAll(r.Body)
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  shortURL := string(bodyBytes[:SHORT_LENGTH])

  err = h.rdb.Del(h.ctx, shortURL, shortURL + ":dates").Err()
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  usrLinksKey := "usr:" + session.Username + ":links"
  err = h.rdb.LRem(h.ctx, usrLinksKey, 0, shortURL).Err()
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

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

func (h RequestHandler) handleHome(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "*")

  shortURL := r.URL.Path[1:]
  longURL, err := getLongFromShortURL(h.ctx, h.rdb, shortURL)
  if err == nil {
    http.Redirect(w, r, longURL, http.StatusFound)
    year, month, date := time.Now().Date()
    dateString := fmt.Sprintf("%d-%d-%d", year, month, date)
    h.rdb.LPush(h.ctx, shortURL + ":dates", dateString)
    return
  }

  staticHandler := http.FileServer(http.Dir("../frontend/build/"))
  staticHandler.ServeHTTP(w, r)
}

func (h RequestHandler) getLinkData(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "/")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  sessionKeyCookie, err := r.Cookie("sessionKey")
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  _, found := h.sessions[sessionKeyCookie.Value]
  if !found {
    http.Error(w, "Invalid session", 401)
    return
  } 

  defer r.Body.Close()
  bodyBytes, err := ioutil.ReadAll(r.Body)
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  shortURL := string(bodyBytes[:SHORT_LENGTH])
  longURL, err := getLongFromShortURL(h.ctx, h.rdb, shortURL)
  if err != nil {
    http.Error(w, err.Error(), 404)
    return
  }

  datesKey := shortURL + ":dates"

  allDates, err := h.rdb.LRange(h.ctx, datesKey, 0, -1).Result()
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  nowYear, nowMonth, nowDay := time.Now().Date()
  var numClicks [30]int
  for i := 0; i < len(allDates); i++ {
    nums := strings.Split(allDates[i], "-")
    if len(nums) != 3 {
      continue
    }
    year, err := strconv.Atoi(nums[0])
    if err != nil {
      continue
    }
    month, err := strconv.Atoi(nums[1])
    if err != nil {
      continue
    }
    day, err := strconv.Atoi(nums[2])
    if err != nil {
      continue
    }

    daysSince := (nowYear - year) * 365 + (int(nowMonth) - int(month)) * 30 + (nowDay - day)
    if daysSince >= 30 {
      break
    }
    numClicks[daysSince] += 1
  }

  data := LinkData{
    LongURL: longURL,
    NumClicks: numClicks,
  }

  jsonBytes, err := json.Marshal(data)
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  w.Write(jsonBytes)
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
  http.HandleFunc("/api/login", requestHandler.handleLogin)
  http.HandleFunc("/api/logout", requestHandler.handleLogout)
  http.HandleFunc("/api/register", requestHandler.handleRegister)
  http.HandleFunc("/api/getLinks", requestHandler.getLinks)
  http.HandleFunc("/api/getLinkData", requestHandler.getLinkData)
  http.HandleFunc("/api/removeLink", requestHandler.removeLink)

  fmt.Println("Listening on http://localhost:8080")
  http.ListenAndServe(":8080", nil)
}
