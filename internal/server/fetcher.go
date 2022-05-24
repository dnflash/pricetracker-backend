package server

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"pricetracker/internal/client"
	"pricetracker/internal/database"
	"time"
)

func (s Server) FetchDataInInterval(ctx context.Context, ticker *time.Ticker) {
	for range ticker.C {
		s.fetchData(ctx)
	}
}

func (s Server) fetchData(ctx context.Context) {
	s.Logger.Info("fetchData: Fetching all item data...")

	is, err := s.DB.ItemsFindAll(ctx)
	if err != nil {
		s.Logger.Errorf("fetchData: Error getting all Items from database, err: %v", err)
		return
	}

	for _, i := range is {
		siteType, cleanURL, err := siteTypeAndCleanURL(i.URL)
		if err != nil {
			s.Logger.Errorf("fetchData: Error getting site type from url: %s, err: %v", i.URL, err)
			continue
		}
		switch siteType {
		case siteShopee:
			shopeeItem, err := s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				s.Logger.Errorf("fetchData: Error getting Shopee item from url: %s, err: %v", cleanURL, err)
				continue
			}
			ih := database.ItemHistory{
				ItemID:    i.ID,
				Price:     shopeeItem.Price,
				Stock:     shopeeItem.Stock,
				Timestamp: primitive.NewDateTimeFromTime(time.Now()),
			}
			if err = s.DB.ItemHistoryInsert(ctx, ih); err != nil {
				s.Logger.Errorf("fetchData: Error inserting ItemHistory, err: %v", err)
				continue
			}
			if shopeeItem.Price != i.Price || shopeeItem.Stock != i.Stock {
				if err = s.DB.ItemPriceAndStockUpdate(ctx, i.ID, shopeeItem.Price, shopeeItem.Stock); err != nil {
					s.Logger.Errorf("fetchData: Error updating Item Price and Stock, err: %v", err)
				}

				us, err := s.DB.UserDeviceFCMTokensFindByTrackedItem(ctx, i.ID)
				if err != nil {
					s.Logger.Errorf("fetchData: Error getting Users that tracked ItemID: %s, err: %v", i.ID.Hex(), err)
					continue
				}

				var notifiedUserIDs []primitive.ObjectID
				var fcmTokens []string

				for _, u := range us {
					if len(u.TrackedItems) > 0 && shouldNotify(u.TrackedItems[0], shopeeItem.Price, shopeeItem.Stock) {
						notifiedUserIDs = append(notifiedUserIDs, u.ID)
						for _, d := range u.Devices {
							if d.FCMToken != "" {
								fcmTokens = append(fcmTokens, d.FCMToken)
							}
						}
					}
				}

				fcmReq := client.FCMSendRequest{
					Notification: client.FCMNotification{
						Title:       "The price of an item has dropped!",
						Body:        fmt.Sprintf("%s is now Rp. %d", shopeeItem.Name, shopeeItem.Price),
						ClickAction: "FLUTTER_NOTIFICATION_CLICK",
						Sound:       "default",
					},
					Data:            client.FCMData{ItemID: i.ID.Hex()},
					RegistrationIDs: fcmTokens,
				}

				s.Logger.Infof("fetchData: Sending notification to %d User(s) for ItemID: %s", len(notifiedUserIDs), i.ID.Hex())
				s.Logger.Debugf("fetchData: FCMSendRequest for ItemID: %s, req: %+v", i.ID.Hex(), fcmReq)

				fcmResp, err := s.Client.FCMSendNotification(fcmReq)
				if err != nil {
					s.Logger.Errorf(
						"fetchData: Error sending notification to FCM for ItemID: %s, FCMSendRequest: %+v, err: %v",
						i.ID.Hex(), fcmReq, err,
					)
					continue
				}

				s.Logger.Infof("fetchData: Send notification results for ItemID: %s, success: %d, failure: %d",
					i.ID.Hex(), fcmResp.Success, fcmResp.Failure)
				s.Logger.Debugf("fetchData: FCMSendResponse for ItemID: %s, resp: %+v", i.ID.Hex(), fcmResp)

				updatedUserCount, err := s.DB.UserTrackedItemNotificationCountIncrement(ctx, notifiedUserIDs, i.ID)
				if err != nil {
					s.Logger.Errorf("fetchData: Error incrementing User TrackedItem Notification Counts, err: %v", err)
					continue
				}
				if updatedUserCount != len(notifiedUserIDs) {
					s.Logger.Errorf(
						"fetchData: Updated User count mismatch with notified UserIDs, updated: %d, notified: %d, notifiedUserIDs: %v",
						updatedUserCount, len(notifiedUserIDs), notifiedUserIDs,
					)
				}
			}
		}
	}

	s.Logger.Info("fetchData: Finished fetching all item data")
}

func shouldNotify(ti database.TrackedItem, itemPrice int, itemStock int) bool {
	if ti.NotificationEnabled && ti.NotificationCount <= 5 &&
		itemPrice <= ti.PriceLowerBound && itemStock > 0 {
		return true
	}
	return false
}
