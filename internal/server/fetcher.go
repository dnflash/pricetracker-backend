package server

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"pricetracker/internal/client"
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
			s.Logger.Debugf("fetchData: Finding Users that tracked Item: %s, ID: %s", itemName, i.ID.Hex())
			us, err := s.DB.UsersDeviceFCMTokensFindByTrackedItem(ctx, i.ID)
			if err != nil {
				s.Logger.Errorf("fetchData: Error getting Users that tracked ItemID: %s, err: %v", i.ID.Hex(), err)
				continue
			}
			s.Logger.Debugf("fetchData: Found %d User(s) that tracked Item: %s, ID: %s", len(us), itemName, i.ID.Hex())

			var notifiedUserIDs []primitive.ObjectID
			var fcmTokens []string
			for _, u := range us {
				if len(u.TrackedItems) > 0 && shouldNotify(u.TrackedItems[0], ecommerceItem.Price, ecommerceItem.Stock) {
					var notified bool
					for _, d := range u.Devices {
						if d.FCMToken != "" {
							fcmTokens = append(fcmTokens, d.FCMToken)
							notified = true
						}
					}
					if notified {
						notifiedUserIDs = append(notifiedUserIDs, u.ID)
					}
				}
			}
			if len(notifiedUserIDs) == 0 {
				s.Logger.Debugf("fetchData: No Users to be notified for Item: %s, ID: %s", itemName, i.ID.Hex())
				continue
			}

			fcmReq := client.FCMSendRequest{
				Notification: client.FCMNotification{
					Title:       "The price of an item has dropped!",
					Body:        fmt.Sprintf("%s is now Rp. %d", itemName, ecommerceItem.Price),
					ClickAction: "FLUTTER_NOTIFICATION_CLICK",
					Sound:       "default",
				},
				Data:            client.FCMData{ItemID: i.ID.Hex()},
				RegistrationIDs: fcmTokens,
			}
			s.Logger.Infof("fetchData: Sending notification to %d Device(s) for %d User(s) for Item: %s, ID: %s",
				len(fcmTokens), len(notifiedUserIDs), itemName, i.ID.Hex())
			s.Logger.Debugf("fetchData: FCMSendRequest for Item: %s, ID: %s, req: %+v", itemName, i.ID.Hex(), fcmReq)
			fcmResp, err := s.Client.FCMSendNotification(fcmReq)
			if err != nil {
				s.Logger.Errorf(
					"fetchData: Error sending notification to FCM for Item: %s, ID: %s, FCMSendRequest: %+v, err: %v",
					itemName, i.ID.Hex(), fcmReq, err,
				)
				continue
			}
			s.Logger.Infof("fetchData: Send notification results for Item: %s, ID: %s, success: %d, failure: %d",
				itemName, i.ID.Hex(), fcmResp.Success, fcmResp.Failure)
			s.Logger.Debugf("fetchData: FCMSendResponse for Item: %s, ID: %s, resp: %+v", itemName, i.ID.Hex(), fcmResp)

			updatedUserCount, err := s.DB.UserTrackedItemNotificationCountIncrement(ctx, notifiedUserIDs, i.ID)
			if err != nil {
				s.Logger.Errorf("fetchData: Error incrementing User TrackedItem Notification Counts, err: %v", err)
				continue
			}
			if updatedUserCount != len(notifiedUserIDs) {
				s.Logger.Errorf(
					"fetchData: Updated User count mismatch with notified UserIDs, updated: %d, notified: %d, notifiedUserIDs: %v for Item: %s, ID: %s",
					updatedUserCount, len(notifiedUserIDs), notifiedUserIDs, itemName, i.ID.Hex(),
				)
			}
		} else {
			s.Logger.Infof("fetchData: No changes on price for Item: %s, ID: %s, will not notify Users", itemName, i.ID.Hex())
			continue
		}

	}
	s.Logger.Info("fetchData: Finished fetching all Item data")
}

func shouldNotify(ti model.TrackedItem, itemPrice int, itemStock int) bool {
	if ti.NotificationEnabled &&
		itemPrice <= ti.PriceLowerThreshold &&
		itemStock > 0 {
		return true
	}
	return false
}
