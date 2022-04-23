package server

import (
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"pricetracker/internal/database"
	"time"
)

func (s Server) FetchDataInInterval(ticker *time.Ticker) {
	for range ticker.C {
		s.fetchData()
	}
}

func (s Server) fetchData() {
	s.Logger.Info("Fetching all item data...")

	is, err := s.DB.ItemFindAll(context.TODO())
	if err != nil {
		s.Logger.Errorf("Error getting all items from database, err: %+v", err)
	}

	for _, i := range is {
		siteType, err := siteTypeFromURL(i.URL)
		if err != nil {
			s.Logger.Errorf("Error getting site type from url: %+v, err: %+v", i.URL, err)
		}
		switch siteType {
		case siteShopee:
			shopeeItem, err := s.Client.ShopeeGetItem(i.URL)
			if err != nil {
				s.Logger.Errorf("Error getting Shopee item from url: %+v, err: %+v", i.URL, err)
			}
			ih := database.ItemHistory{
				ItemID:    i.ID,
				Price:     shopeeItem.Price,
				Stock:     shopeeItem.Stock,
				Timestamp: primitive.NewDateTimeFromTime(time.Now()),
			}
			if err = s.DB.ItemHistoryInsert(context.TODO(), ih); err != nil {
				s.Logger.Errorf("Error inserting ItemHistory: %+v, err: %+v", ih, err)
			}
		}
	}

	s.Logger.Info("Finished fetching all item data")
}
