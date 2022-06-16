package server

import (
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"pricetracker/internal/model"
	"time"
)

func (s Server) FetchDataInInterval(ctx context.Context, ticker *time.Ticker) {
	for range ticker.C {
		s.fetchData(ctx)
	}
}

func (s Server) fetchData(ctx context.Context) {
	s.Logger.Info("fetchData: Starting to fetch all Item data")
	is, err := s.DB.ItemsFindAll(ctx)
	if err != nil {
		s.Logger.Errorf("fetchData: Error getting all Items from DB, err: %v", err)
		return
	}
	s.Logger.Infof("fetchData: Retrieved %d Item(s) from DB", len(is))

	for _, i := range is {
		time.Sleep(300 * time.Millisecond)
		var itemName string
		if len(i.Name) > 45 {
			itemName = i.Name[:45] + "..."
		} else {
			itemName = i.Name
		}
		s.Logger.Infof("fetchData: Fetching data for Item: %s, ID: %s", itemName, i.ID.Hex())
		siteType, cleanURL, err := siteTypeAndCleanURL(i.URL)
		if err != nil {
			s.Logger.Errorf("fetchData: Error getting site type from url: %s, err: %v", i.URL, err)
			continue
		}
		var ecommerceItem model.Item
		switch siteType {
		case siteShopee:
			s.Logger.Debugf("fetchData: Getting Item data from Shopee for Item: %s, ID: %s", itemName, i.ID.Hex())
			ecommerceItem, err = s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				s.Logger.Errorf("fetchData: Error getting Shopee item from url: %s, err: %v", cleanURL, err)
				continue
			}
		}

		s.Logger.Debugf("fetchData: Updating Item: %s, ID: %s", itemName, i.ID.Hex())
		updatedI := i
		updatedI.UpdateWith(ecommerceItem)
		if err = s.DB.ItemUpdate(ctx, updatedI); err != nil {
			s.Logger.Errorf("fetchData: Error updating Item, err: %v", err)
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

		if ecommerceItem.Price != i.Price {
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
