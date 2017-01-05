package main

import (
  "encoding/json"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "net/url"
  "os"
  "os/user"
  "path/filepath"
  "time"
  "bufio"

  "golang.org/x/net/context"
  "golang.org/x/oauth2"
  "golang.org/x/oauth2/google"
  "google.golang.org/api/calendar/v3"

  "github.com/PuloV/ics-golang"
  "github.com/BurntSushi/toml"
  "github.com/headzoo/surf"
)

const APP_NAME string = "terrasync"
const TERM_MONTH int = 3

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
  cacheFile, err := tokenCacheFile()
  if err != nil {
    log.Fatalf("Unable to get path to cached credential file. %v", err)
  }
  tok, err := tokenFromFile(cacheFile)
  if err != nil {
    tok = getTokenFromWeb(config)
    saveToken(cacheFile, tok)
  }
  return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
  authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
  fmt.Printf("Go to the following link in your browser then type the "+
    "authorization code: \n%v\n", authURL)

  var code string
  if _, err := fmt.Scan(&code); err != nil {
    log.Fatalf("Unable to read authorization code %v", err)
  }

  tok, err := config.Exchange(oauth2.NoContext, code)
  if err != nil {
    log.Fatalf("Unable to retrieve token from web %v", err)
  }
  return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
  usr, err := user.Current()
  if err != nil {
    return "", err
  }
  tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
  os.MkdirAll(tokenCacheDir, 0700)
  return filepath.Join(tokenCacheDir,
    url.QueryEscape(APP_NAME + ".json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
  f, err := os.Open(file)
  if err != nil {
    return nil, err
  }
  t := &oauth2.Token{}
  err = json.NewDecoder(f).Decode(t)
  defer f.Close()
  return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
  fmt.Printf("Saving credential file to: %s\n", file)
  f, err := os.Create(file)
  if err != nil {
    log.Fatalf("Unable to cache oauth token: %v", err)
  }
  defer f.Close()
  json.NewEncoder(f).Encode(token)
}

// delete events
func deleteOldEvents(start string, end string, srv *calendar.Service) {

  oldEvents, err := srv.Events.List(config.Google.CalendarId).ShowDeleted(false).
  SingleEvents(true).TimeMin(start).TimeMax(end).OrderBy("startTime").Do()
  if err != nil {
    log.Fatalf("Unable to retrieve next ten of the user's events. %v", err)
  }

  fmt.Println("Delete events:")

  if len(oldEvents.Items) > 0 {
    for _, i := range oldEvents.Items {
      srv.Events.Delete(config.Google.CalendarId, i.Id).Do()
      fmt.Printf("del %s \n", i.Id)
    }
  } else {
    fmt.Printf("No deletable events found.\n")
  }

}

// insert events
func insertNewEvents(icsFile string, termStart string, termEnd string, srv *calendar.Service) {

  fmt.Println("Insert events:")

  parser := ics.New()
  inputChan := parser.GetInputChan()
  inputChan <- icsFile
  parser.Wait()

  newCalendars, err := parser.GetCalendars()

  if err == nil && len(newCalendars) > 0 {
    for _, e := range newCalendars[0].GetEvents() {

      ts, err := time.Parse(time.RFC3339, termStart)
      te, err := time.Parse(time.RFC3339, termEnd)

      if !e.GetStart().Before(ts) && !e.GetStart().After(te) { // ts <= e.GetStart() && e.GetStart() <= te

        newEvent := &calendar.Event{
          Summary: e.GetSummary(),
          Location: e.GetLocation(),
          Description: e.GetDescription(),
          Start: &calendar.EventDateTime{
            DateTime: e.GetStart().Format(time.RFC3339),
          },
          End: &calendar.EventDateTime{
            DateTime: e.GetEnd().Format(time.RFC3339),
          },
        }

        newEvent, err = srv.Events.Insert(config.Google.CalendarId, newEvent).Do()

        if err != nil {
          log.Fatalf("Unable to create event. %v\n", err)
        }
        fmt.Printf("Event created: %s\n", newEvent.HtmlLink)
        //fmt.Printf("%s %s %s %s %s %s\n", newEvent.GetSummary(), newEvent.GetStart().Format(time.RFC3339),
        //  newEvent.GetEnd().Format(time.RFC3339), newEvent.GetLocation(), newEvent.GetDescription(), newEvent.GetWholeDayEvent())
      } else {
        fmt.Printf("skip\n")

      }
    }

  } else {
    fmt.Printf("No insertable events found.\n")
    fmt.Println(err)
  }
}

// calc term
func calcTerm(now time.Time, term_month int, srv *calendar.Service) (string, string) {

  start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format(time.RFC3339)
  end := time.Date(now.Year(), now.Month() + time.Month(term_month), 1, 23, 59, 59, 0, time.Local).AddDate(0, 0, -1).Format(time.RFC3339)

  fmt.Printf("from %s to %s\n", start, end)

  return start, end
}

func terra() {
  // Create a new browser and open.
  bow := surf.NewBrowser()
  err := bow.Open(config.Terra.LoginUrl)
  if err != nil {
      panic(err)
  }

  // Log in to the site.
  fm, _ := bow.Form("form#login_form")
  fm.Input("username", config.Terra.Username)
  fm.Input("password", config.Terra.Password)
  if fm.Submit() != nil {
      panic(err)
  }

  err = bow.Open(config.Terra.DownloadUrl)

  filename := "./tmp/hoge.csv"
  f, err := os.Create(filename)
  if err != nil {
      log.Printf("Error creating file '%s'.", filename)
  }
  defer f.Close()

  csv := bow.Body()
  writer := bufio.NewWriter(f)

  _, err = writer.WriteString(csv)
  if err != nil {
      log.Fatal(err)
  }
  writer.Flush()

}


type Config struct {
  Google GoogleConfig
  Terra  TerraConfig
}

type GoogleConfig struct {
  CalendarId string `toml:"calendar_id"`
}

type TerraConfig struct {
  LoginUrl string `toml:"login_url"`
  DownloadUrl string `toml:"download_url"`
  Username string `toml:"username"`
  Password string `toml:"password"`
}

var config Config

func main() {

  _, err := toml.DecodeFile("config.tml", &config)
  if err != nil {
      panic(err)
  }
  fmt.Printf("CalendarId is :%s\n", config.Google.CalendarId)

  // set proxies
  //os.Setenv("HTTP_PROXY", "")
  //os.Setenv("HTTPS_PROXY", "")

  ctx := context.Background()

  b, err := ioutil.ReadFile("client_secret.json")
  if err != nil {
    log.Fatalf("Unable to read client secret file: %v", err)
  }

  // If modifying these scopes, delete your previously saved credentials
  // at ~/.credentials/calendar-go-quickstart.json
  config, err := google.ConfigFromJSON(b, calendar.CalendarScope)
  if err != nil {
    log.Fatalf("Unable to parse client secret file to config: %v", err)
  }
  client := getClient(ctx, config)

  srv, err := calendar.New(client)
  if err != nil {
    log.Fatalf("Unable to retrieve calendar Client %v", err)
  }

  start, end := calcTerm(time.Now(), TERM_MONTH, srv)

  terra()
  deleteOldEvents(start, end, srv)
  insertNewEvents("./tmp/schedule.ics", start, end, srv)

}
