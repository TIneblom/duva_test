package main

import (
	"fmt"
	"context"
	"net/http"
  "github.com/redis/go-redis/v9"
  "golang.org/x/crypto/bcrypt"
  "math/rand"
)

func registerUser(ctx context.Context, rdb *redis.Client, username string, password string) error {
	usrKey := "usr:" + username

  exists, err := rdb.Exists(ctx, usrKey).Result()
  if exists == 1 || err != nil {
    return fmt.Errorf("User already exists")
  } 

  hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
  if err != nil {
    return err
  }

  err = rdb.Set(ctx, usrKey, string(hash), 0).Err()
  if err != nil {
    return err
  }

  return err
}

func loginUser(ctx context.Context, rdb *redis.Client, sessions map[string]Session, username string, password string) (*http.Cookie, error) {
	hash, err := rdb.Get(ctx, "usr:" + username).Result()
  if err != nil {
    return nil, fmt.Errorf("User not found")
  }

  err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
  if err == nil {
    for key, session := range sessions {
      if username == session.Username {
        delete(sessions, key)
        break
      }
    } 

    const cookieChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!#$%&'*+-./:<=>?@^_`{|}~"
    sessionKey := make([]byte, 32)
    for {
      for i := 0; i < 32; i++ {
        sessionKey[i] = byte(cookieChars[rand.Intn(len(cookieChars))])
      }
      if _, found := sessions[string(sessionKey)]; !found {
        break
      }
    }

    sessions[string(sessionKey)] = Session{
      Username: username,
    }

    cookie := &http.Cookie{
      Name: "sessionKey",
      Value: string(sessionKey),
      Path: "/",
      HttpOnly: true,
    }
    return cookie, err
  } else {
    return nil, fmt.Errorf("Invalid password")
  }
  return nil, err
}

func logoutUser(sessions map[string]Session, sessionKey string) {
  delete(sessions, sessionKey)
}