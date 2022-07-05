package server

import (
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"math/rand"
	"pricetracker/internal/model"
	"time"
)

func (s Server) FetchDataInInterval(ctx context.Context, interval time.Duration) {

	tickerShopee := time.NewTicker(interval)
	tickerTokopedia := time.NewTicker(interval)
	tickerBlibli := time.NewTicker(interval)
	go func() {
		for range tickerShopee.C {
			s.Logger.Info("fetchData: Starting to fetch all Shopee Items")
			if is, err := s.DB.ItemsFindWithSite(ctx, "Shopee"); err != nil {
				s.Logger.Errorf("fetchData: Error getting all Shopee Items from DB, err: %v", err)
				continue
			} else {
				s.Logger.Infof("fetchData: Retrieved %d Shopee Item(s) from DB", len(is))
				s.fetchData(ctx, is)
			}
		}
	}()
	time.Sleep(3 * time.Second)
	go func() {
		for range tickerTokopedia.C {
			s.Logger.Info("fetchData: Starting to fetch all Tokopedia Items")
			if is, err := s.DB.ItemsFindWithSite(ctx, "Tokopedia"); err != nil {
				s.Logger.Errorf("fetchData: Error getting all Tokopedia Items from DB, err: %v", err)
				continue
			} else {
				s.Logger.Infof("fetchData: Retrieved %d Tokopedia Item(s) from DB", len(is))
				s.fetchData(ctx, is)
			}
		}
	}()
	time.Sleep(5 * time.Second)
	go func() {
		for range tickerBlibli.C {
			s.Logger.Info("fetchData: Starting to fetch all Blibli Items")
			if is, err := s.DB.ItemsFindWithSite(ctx, "Blibli"); err != nil {
				s.Logger.Errorf("fetchData: Error getting all Blibli Items from DB, err: %v", err)
				continue
			} else {
				s.Logger.Infof("fetchData: Retrieved %d Blibli Item(s) from DB", len(is))
				s.fetchData(ctx, is)
			}
		}
	}()
}

func (s Server) fetchData(ctx context.Context, is []model.Item) {
	rand.Seed(time.Now().UnixNano())
	for _, i := range is {
		time.Sleep(10 * time.Second)
		time.Sleep(time.Duration(rand.Intn(10)) * time.Second)

		var itemName string
		if len(i.Name) > 45 {
			itemName = i.Name[:45] + "..."
		} else {
			itemName = i.Name
		}
		s.Logger.Infof("fetchData: Fetching data for Item: %s, ID: %s", itemName, i.ID.Hex())
		urlSiteType, cleanURL, err := siteTypeAndCleanURL(i.URL)
		if err != nil {
			s.Logger.Errorf("fetchData: Error getting site type from url: %s, err: %v", i.URL, err)
			continue
		}
		var ecommerceItem model.Item
		switch urlSiteType {
		case siteShopee:
			s.Logger.Debugf("fetchData: Getting Item data from Shopee for Item: %s, ID: %s", itemName, i.ID.Hex())
			ecommerceItem, err = s.Client.ShopeeGetItem(cleanURL, false)
			if err != nil {
				s.Logger.Errorf("fetchData: Error getting Shopee item from url: %s, err: %v", cleanURL, err)
				continue
			}
		case siteTokopedia:
			s.Logger.Debugf("fetchData: Getting Item data from Tokopedia for Item: %s, ID: %s", itemName, i.ID.Hex())
			ecommerceItem, err = s.Client.TokopediaGetItem(cleanURL, false)
			if err != nil {
				s.Logger.Errorf("fetchData: Error getting Tokopedia item from url: %s, err: %v", cleanURL, err)
				continue
			}
		case siteBlibli:
			s.Logger.Debugf("fetchData: Getting Item data from Blibli for Item: %s, ID: %s", itemName, i.ID.Hex())
			ecommerceItem, err = s.Client.BlibliGetItem(cleanURL, false)
			if err != nil {
				s.Logger.Errorf("fetchData: Error getting Blibli item from url: %s, err: %v", cleanURL, err)
				continue
			}
		}

		s.Logger.Debugf("fetchData: Updating Item: %s, ID: %s", itemName, i.ID.Hex())
		updatedI := i
		updatedI.UpdateWith(ecommerceItem)
		if err = s.DB.ItemUpdate(ctx, updatedI); err != nil {
			s.Logger.Errorf("fetchData: Error updating Item, err: %v", err)
		}

		lastIH, err := s.DB.ItemHistoryFindLatest(ctx, i.ID.Hex())
		if err != nil {
			s.Logger.Errorf("fetchData: Error getting latest ItemHistory for Item: %s, ID: %s, err: %v", itemName, i.ID.Hex(), err)
			continue
		}

		s.Logger.Debugf("fetchData: Inserting ItemHistory for Item: %s, ID: %s", itemName, i.ID.Hex())
		ih := model.ItemHistory{
			ItemID:    i.ID,
			Price:     ecommerceItem.Price,
			Stock:     ecommerceItem.Stock,
			Rating:    ecommerceItem.Rating,
			Sold:      ecommerceItem.Sold,
			Timestamp: primitive.NewDateTimeFromTime(time.Now()),
		}
		if err = s.DB.ItemHistoryInsert(ctx, ih); err != nil {
			s.Logger.Errorf("fetchData: Error inserting ItemHistory, err: %v", err)
		}

		if ecommerceItem.Price != lastIH.Price {
			if ecommerceItem.Stock == 0 {
				s.Logger.Debugf("fetchData: Stock is 0 for Item: %s, ID: %s, will not notify Users", itemName, i.ID.Hex())
				continue
			}
			s.Logger.Infof("fetchData: Price changed, notifying Users for Item: %s, ID: %s", itemName, i.ID.Hex())
			s.notify(ctx, updatedI)
		} else {
			s.Logger.Infof("fetchData: No changes on price for Item: %s, ID: %s, will not notify Users", itemName, i.ID.Hex())
			continue
		}
	}
	s.Logger.Info("fetchData: Finished fetching all Item data")
}
