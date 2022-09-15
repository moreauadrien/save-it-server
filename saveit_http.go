package function

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/PuerkitoBio/goquery"
)

type PageDetails struct {
	title   string
	image   string
	favicon string
	url     string
}

type ReceivedPayload struct {
	IntegrationToken string `json:"integrationToken"`
	DatabaseId       string `json:"databaseId"`
	Url              string `json:"url"`
	Tags             string `json:"tags"`
}

func init() {
	functions.HTTP("SaveIt", saveIt)
}

func formatTags(rawTags string) string {
	tags := strings.Split(rawTags, ",")
	for i := 0; i < len(tags); i++ {
		tag := strings.Trim(tags[i], " ")
		tags[i] = `{"name":"` + tag + `"}`
	}

	return "[" + strings.Join(tags, ",") + "]"
}

func getPageDetails(u string) (PageDetails, error) {
	details := PageDetails{}

	res, err := http.Get(u)
	if err != nil {
		return details, fmt.Errorf("The page could not be fetched")
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		return details, fmt.Errorf("The page return a %d error", res.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return details, err
	}

	favicon, err := getFaviconUrl(u)
	if err != nil {
		return details, err
	}

	details.favicon = favicon

	image, _ := doc.Find("meta[property='og:image']").Attr("content")
	details.image = image

	title := doc.Find("title").Text()
	details.title = title

	details.url = u

	return details, nil
}

func getFaviconUrl(u string) (string, error) {
	o, err := url.Parse(u)
	if err != nil {
		return "", err
	}

	host := strings.Split(o.Host, ":")[0]

	return "https://icon.horse/icon/" + host, nil
}

func getNotionPayload(details PageDetails, tags string, rPayload ReceivedPayload) *bytes.Buffer {
	pageContent := ""
	if len(details.image) > 0 {
		pageContent = `"children": [
                {
                    "object": "block",
                    "image": {
                        "external": {
                            "url": "` + details.image + `"
                        }
                    }
                    
                }
            ],`
	}
	notionPayload := []byte(`{
        "parent": {
            "database_id": "` + rPayload.DatabaseId + `"
        },
        "icon": {
            "external": {
                "url": "` + details.url + `"
            }
        },
        ` + pageContent + `
        "properties": {
            "Name": {
                "title": [
                    {
                        "text": {
                            "content": "` + details.title + `"
                        }
                    }
                ]
            },
            "Tags": {
                "multi_select": ` + tags + `
            },
            "URL": {
                "url": "` + details.url + `"
            }

        }
    }`)

	return bytes.NewBuffer(notionPayload)
}

func saveIt(w http.ResponseWriter, r *http.Request) {
	d := ReceivedPayload{}

	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, "{\"error\": \"The body could not be parsed\"}", http.StatusBadRequest)
		return
	}

	tags := formatTags(d.Tags)

	details, err := getPageDetails(d.Url)

	if err != nil {
		http.Error(w, "{\"error\": \""+err.Error()+"\"}", http.StatusBadRequest)
		return
	}

	notionPayload := getNotionPayload(details, tags, d)

	req, err := http.NewRequest("POST", "https://api.notion.com/v1/pages", notionPayload)
	req.Header.Set("Authorization", "Bearer "+d.IntegrationToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		http.Error(w, "{\"error\": \""+err.Error()+"\"}", http.StatusBadRequest)
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		http.Error(w, string(body), http.StatusBadRequest)
	}

	fmt.Fprintf(w, string(body))
}
