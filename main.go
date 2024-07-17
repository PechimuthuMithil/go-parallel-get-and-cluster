package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type content struct {
	Num        int    `json:"num"`
	Day        string `json:"day"`
	Month      string `json:"month"`
	Year       string `json:"year"`
	Title      string `json:"title"`
	Transcript string `json:"transcript"`
}

type centre struct {
	Titleset      map[string]struct{} `json:"title-set"`
	Transcriptset map[string]struct{} `json:"transcript-set"`
	Num           int                 `json:"num"`
}

type cluster struct {
	Centre  centre    `json:"centre"`
	Members []content `json:"members"`
}

const OFFLINE_INDEX string = "./clusters"
const MAX_NUM int = 2024

var (
	successfulDownloads int
	clusters            []cluster
	mu                  sync.Mutex
	wg                  sync.WaitGroup
)

func textToSet(text string) map[string]struct{} {
	words := strings.Fields(text)
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		set[word] = struct{}{}
	}
	return set
}

func jaccardSimilarity(set1, set2 map[string]struct{}) float64 {
	intersection := 0
	for word := range set1 {
		if _, found := set2[word]; found {
			intersection++
		}
	}
	union := len(set1) + len(set2) - intersection
	return float64(intersection) / float64(union)
}

func saveCluster(cluster cluster) error {
	filePath := filepath.Join(OFFLINE_INDEX, fmt.Sprintf("cluster%d.json", cluster.Centre.Num))
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(cluster)
}

func get_and_cluster(url string) {
	defer wg.Done()

	start := time.Now()
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[-] %s failed: %v\n", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var comic content
		if err := json.NewDecoder(resp.Body).Decode(&comic); err != nil {
			log.Printf("[-] %s failed to decode: %v\n", url, err)
			return
		}

		duration := time.Since(start).Seconds()
		log.Printf("[+] %s took %.2f seconds to download\n", url, duration)

		mu.Lock()
		defer mu.Unlock()

		// Increment the successful download count
		successfulDownloads++

		titleSet := textToSet(comic.Title)
		transcriptSet := textToSet(comic.Transcript)
		foundCluster := false

		for i, cluster := range clusters {
			centreTitleSet := cluster.Centre.Titleset
			centreTranscriptSet := cluster.Centre.Transcriptset

			if jaccardSimilarity(titleSet, centreTitleSet) >= 0.2 || jaccardSimilarity(transcriptSet, centreTranscriptSet) >= 0.2 {
				clusters[i].Members = append(clusters[i].Members, comic)
				foundCluster = true
				break
			}
		}

		if !foundCluster {
			newCluster := cluster{
				Centre:  centre{Titleset: titleSet, Transcriptset: transcriptSet, Num: comic.Num},
				Members: []content{comic},
			}
			clusters = append(clusters, newCluster)
		}

		for _, cluster := range clusters {
			if err := saveCluster(cluster); err != nil {
				log.Printf("[-] Failed to save cluster: %v\n", err)
			}
		}
	} else {
		log.Printf("[-] %s returned status code: %d\n", url, resp.StatusCode)
	}
}

func main() {
	// Ensure the directory exists
	if err := os.MkdirAll(OFFLINE_INDEX, os.ModePerm); err != nil {
		log.Fatalf("[-] Failed to create directory %s: %v\n", OFFLINE_INDEX, err)
	}

	list := []string{}
	for i := 1; i <= MAX_NUM; i++ {
		list = append(list, fmt.Sprintf("https://xkcd.com/%d/info.0.json", i))
	}

	for _, url := range list {
		wg.Add(1)
		go get_and_cluster(url)
	}

	wg.Wait()
	log.Printf("Total successful downloads: %d\n", successfulDownloads)
}
