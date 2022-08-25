package webhook

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	reportListSize   = 1
	postTimeThrottle = 1 //minutes
)

type Webhook struct {
	domain           string
	hookURLs         []string
	slackTime        time.Time
	mux              sync.Mutex
	postTimeThrottle float64

	reportCount uint32
	dupeCount   uint32
	reportMinId uint64
	reportMaxId uint64
	reportList  []ReportListEntry
}

func New(domain string, hookURLs []string, postTimeThrottle uint32) *Webhook {
	hook := &Webhook{
		domain:           domain,
		hookURLs:         hookURLs,
		slackTime:        time.Now(),
		reportList:       make([]ReportListEntry, 0, reportListSize),
		postTimeThrottle: float64(postTimeThrottle),
	}
	return hook
}

func (w *Webhook) BumpDupeCounter() {
	w.mux.Lock()
	w.dupeCount += 1
	w.mux.Unlock()
}

func (w *Webhook) Post(entry ReportListEntry) {
	w.mux.Lock()
	defer w.mux.Unlock()

	if w.reportMinId == 0 {
		w.reportMinId = entry.ReportId
	}
	if w.reportMaxId < entry.ReportId {
		w.reportMaxId = entry.ReportId
	}

	w.reportCount += 1
	if len(w.reportList) < cap(w.reportList) {
		w.reportList = append(w.reportList, entry)
	}

	listUrl := fmt.Sprintf("%s/list?min=%d&max=%d", w.domain, w.reportMinId, w.reportMaxId)

	messageText := make([]string, 0, 20)
	for _, entry := range w.reportList {
		messageText = append(messageText, fmt.Sprintf("[#%d: %s](%s/view/%d)", entry.ReportId, entry.Message, w.domain, entry.ReportId))
	}
	t := strings.Join(messageText, "\n")
	if w.reportCount > uint32(cap(w.reportList)) {
		t += fmt.Sprintf("\n\n%d more reports not shown. <%s|View the full list>", w.reportCount-uint32(cap(w.reportList)), listUrl)
	}

	data := &slackMessage{
		Content: "<@803263987520241704> <@264465713379409921>",
		Attachments: []slackAttachment{
			{
				Title:       "Hata oluştu",
				URL:         listUrl,
				Color:       16711680,
				Description: t,
			},
		},
	}
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.Encode(data)
	fmt.Println(buf.String())
	encoded := buf.Bytes()

	for _, webhookURL := range w.hookURLs {
		encodedCopy := make([]byte, len(encoded))
		copy(encodedCopy, encoded)
		req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(encodedCopy))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("error happened when posting to webhook: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != 204 {
			log.Printf("error happened posting update to webhook %s", webhookURL)
			log.Println(hex.Dump(buf.Bytes()))
			log.Println("response Status:", resp.Status)
			log.Println("response Headers:", resp.Header)
			body, _ := ioutil.ReadAll(resp.Body)
			log.Println("response Body:", string(body))
		} else {
			log.Printf("posted update to webhook %s successfully", webhookURL)
		}
	}

	w.reportCount = 0
	w.dupeCount = 0
	w.reportMinId = 0
	w.reportMaxId = 0
	w.reportList = make([]ReportListEntry, 0, reportListSize)
	w.slackTime = time.Now()
}

type ReportListEntry struct {
	ReportId uint64
	Message  string
}

type slackMessage struct {
	Content     string            `json:"content"`
	Attachments []slackAttachment `json:"embeds"`
}

type slackAttachment struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}
