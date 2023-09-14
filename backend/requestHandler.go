package main


import (
	"fmt"
  "io/ioutil"
	"net/http"
	"context"
  "encoding/json"
  "time"
  "strings"
  "strconv"
  "github.com/redis/go-redis/v9"
)

type RequestHandler struct {
  ctx context.Context
  rdb *redis.Client
  sessions map[string]Session
}

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

func (h RequestHandler) getSessionUsername(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "/")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  if r == nil {
    http.Error(w, "Request not found", 400)
  	return
  }

  sessionKeyCookie, err := r.Cookie("sessionKey")
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  session, found := h.sessions[sessionKeyCookie.Value]
  if !found {
  	http.Error(w, "Session not found", 401)
  	return
  }

  fmt.Fprintf(w, session.Username)
}

// registerUser, loginUser, logoutUser finns i auth.go

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

  err = registerUser(h.ctx, h.rdb, body.Username, body.Password)
  if err != nil {
  	http.Error(w, err.Error(), 500)
  	return
  }
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

  cookie, err := loginUser(h.ctx, h.rdb, h.sessions, body.Username, body.Password)
  if err != nil {
  	http.Error(w, err.Error(), 401)
  	return
  }

  http.SetCookie(w, cookie)
}

func (h RequestHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Access-Control-Allow-Origin", "/")
  w.Header().Set("Access-Control-Allow-Credentials", "true")

  sessionKeyCookie, err := r.Cookie("sessionKey")
  if err != nil {
    http.Error(w, err.Error(), 400)
    return
  }

  logoutUser(h.sessions, sessionKeyCookie.Value)
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