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
  "github.com/headzoo/surf/agent"
)

// config setting

const APP_NAME string = "terrasync"
const ICS_FILE_NAME = "./tmp/schedule.ics"

type Config struct {
  Google GoogleConfig
  Terra  TerraConfig
}

type GoogleConfig struct {
  CalendarId string `toml:"calendar_id"`
  DeleteTermMonth int `toml:"delete_term_month"`
}

type TerraConfig struct {
  UserId string `toml:"user_id"`
  UserName string `toml:"user_name"`
  Password string `toml:"password"`
}

var config Config

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
func deleteGCalOldEvents(start string, end string, srv *calendar.Service) {

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
func insertGCalNewEvents(icsFile string, termStart string, termEnd string, srv *calendar.Service) {

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
func calcTerm(now time.Time, termMonth int, srv *calendar.Service) (string, string) {

  start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format(time.RFC3339)
  end := time.Date(now.Year(), now.Month() + time.Month(termMonth), 1, 23, 59, 59, 0, time.Local).AddDate(0, 0, -1).Format(time.RFC3339)

  fmt.Printf("from %s to %s\n", start, end)

  return start, end
}

func downloadTerraSchedule(icsFile string) {

  // Unset proxies
  prevHttpProxy := os.Getenv("HTTP_PROXY")
  prevHttpsProxy := os.Getenv("HTTPS_PROXY")
  os.Setenv("HTTP_PROXY", "")
  os.Setenv("HTTPS_PROXY", "")

  // Create a new browser and open.
  bow := surf.NewBrowser()
  bow.SetUserAgent(agent.Chrome())

  err := bow.Open("http://terra.intra.tis.co.jp/aqua/")
  if err != nil {
      panic(err)
  }

  // Log in to the site.
  fm, err := bow.Form("form[name=loginForm]")

  if err != nil {
      log.Printf("Error access form '%s'.", err)
  }

  fm.Input("loginName", config.Terra.UserName)
  fm.Input("password", config.Terra.Password)

  if fm.Submit() != nil {
      panic(err)
  }

  err = bow.Open("http://terra.intra.tis.co.jp/aqua/" + config.Terra.UserId + "/schedule/view?aqua_format=ical&exa=ical")

  f, err := os.Create(icsFile)
  if err != nil {
      log.Printf("Error creating file '%s'.", icsFile)
  }
  defer f.Close()

  csv := bow.Body()
  writer := bufio.NewWriter(f)

  _, err = writer.WriteString(csv)
  if err != nil {
      log.Fatal(err)
  }
  writer.Flush()

  // Unset proxies
  os.Setenv("HTTP_PROXY", prevHttpProxy)
  os.Setenv("HTTPS_PROXY", prevHttpsProxy)

  fmt.Println("Downloaded.")

}

func main() {

  // config
  _, err := toml.DecodeFile("config.tml", &config)
  if err != nil {
      panic(err)
  }

  // client_secret
  b, err := ioutil.ReadFile("client_secret.json")
  if err != nil {
    log.Fatalf("Unable to read client secret file: %v", err)
  }

  // If modifying these scopes, delete your previously saved credentials
  // at ~/.credentials/terrasync.json
  conf, err := google.ConfigFromJSON(b, calendar.CalendarScope)
  if err != nil {
    log.Fatalf("Unable to parse client secret file to config: %v", err)
  }

  // Calendar Service
  ctx := context.Background()
  client := getClient(ctx, conf)
  srv, err := calendar.New(client)
  if err != nil {
    log.Fatalf("Unable to retrieve calendar Client %v", err)
  }


  downloadTerraSchedule(ICS_FILE_NAME)

  start, end := calcTerm(time.Now(), config.Google.DeleteTermMonth, srv)
  deleteGCalOldEvents(start, end, srv)
  insertGCalNewEvents(ICS_FILE_NAME, start, end, srv)

}
