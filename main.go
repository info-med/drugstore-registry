package main

import (
	"fmt"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
	"github.com/google/uuid"
	"github.com/meilisearch/meilisearch-go"
	"strconv"
	"strings"
	"sync"
	"time"
)

type drugstore struct {
	Id           string
	Address      string
	Name         string
	Municipality string
}

func main() {
	meilisearchClient := meilisearch.NewClient(meilisearch.ClientConfig{
		Host: "http://127.0.0.1:7700",
	})
	wg := sync.WaitGroup{}
	queue := make(chan struct{}, 20)

	for i := 1; i < 118; i++ {
		queue <- struct{}{}
		wg.Add(1)
		pageUrl := "https://lekovi.zdravstvo.gov.mk/pharmacies.grid.pager/" + strconv.Itoa(i) + "/grid_5e6068f0d6265_0"
		fmt.Println("Querying page", i)
		go search(pageUrl, meilisearchClient, &wg, queue)
	}

	wg.Wait()
	close(queue)
}

func search(pageUrl string, meilisearchClient *meilisearch.Client, wg *sync.WaitGroup, queue chan struct{}) {
	defer func() {
		<-queue
		wg.Done()
	}()

	var drugstores []drugstore
	c := colly.NewCollector(
		colly.Async(true),
	)
	c.SetRequestTimeout(60 * time.Second)
	c.OnError(func(resp *colly.Response, err error) {
		fmt.Println(err)
		resp.Request.Retry()
	})
	extensions.RandomUserAgent(c)
	err := c.Limit(&colly.LimitRule{
		DomainRegexp: `lekovi.zdravstvo.gov\.mk`,
		RandomDelay:  5 * time.Second,
		Parallelism:  12,
	})
	if err != nil {
		fmt.Println(err)
	}

	c.OnHTML("tbody", func(h *colly.HTMLElement) {
		h.ForEach("tr", func(_ int, el *colly.HTMLElement) {
			drugstores = append(drugstores, drugstore{
				Id:           uuid.NewString(),
				Name:         strings.Title(strings.ToLower(el.ChildText(".name a"))),
				Address:      el.ChildText(".address"),
				Municipality: el.ChildText(".municipality"),
			})
		})
	})

	c.Visit(pageUrl)
	c.Wait()

	saveToMeilisearch(drugstores, meilisearchClient)
}

func saveToMeilisearch(drugstores []drugstore, meilisearchClient *meilisearch.Client) {
	drugstoreRegistry := meilisearchClient.Index("drugstore-registry")

	_, err := drugstoreRegistry.AddDocuments(drugstores)
	if err != nil {
		fmt.Println(err)
		panic("Error")
	}
}
