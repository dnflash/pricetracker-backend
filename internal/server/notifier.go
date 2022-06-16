package server

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"pricetracker/internal/client"
	"pricetracker/internal/model"
)

func (s Server) notify(ctx context.Context, i model.Item) {
	var itemName string
	if len(i.Name) > 45 {
		itemName = i.Name[:45] + "..."
	} else {
		itemName = i.Name
	}
	s.Logger.Debugf("notify: Finding Users that tracked Item: %s, ID: %s", itemName, i.ID.Hex())
	us, err := s.DB.UsersDeviceFCMTokensFindByTrackedItem(ctx, i.ID)
	if err != nil {
		s.Logger.Errorf("notify: Error getting Users that tracked ItemID: %s, err: %v", i.ID.Hex(), err)
		return
	}
	s.Logger.Debugf("notify: Found %d User(s) that tracked Item: %s, ID: %s", len(us), itemName, i.ID.Hex())

	var notifiedUserIDs []primitive.ObjectID
	var fcmTokens []string
	for _, u := range us {
		if len(u.TrackedItems) > 0 && shouldNotify(u.TrackedItems[0], i.Price, i.Stock) {
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
		s.Logger.Debugf("notify: No Users to be notified for Item: %s, ID: %s", itemName, i.ID.Hex())
		return
	}

	fcmReq := client.FCMSendRequest{
		Notification: client.FCMNotification{
			Title:       "The price of an item has dropped!",
			Body:        fmt.Sprintf("%s is now Rp. %d", itemName, i.Price),
			ClickAction: "FLUTTER_NOTIFICATION_CLICK",
			Sound:       "default",
		},
		Data:            client.FCMData{ItemID: i.ID.Hex()},
		RegistrationIDs: fcmTokens,
	}
	s.Logger.Infof("notify: Sending notification to %d Device(s) for %d User(s) for Item: %s, ID: %s",
		len(fcmTokens), len(notifiedUserIDs), itemName, i.ID.Hex())
	s.Logger.Debugf("notify: FCMSendRequest for Item: %s, ID: %s, req: %+v", itemName, i.ID.Hex(), fcmReq)
	fcmResp, err := s.Client.FCMSendNotification(fcmReq)
	if err != nil {
		s.Logger.Errorf(
			"notify: Error sending notification to FCM for Item: %s, ID: %s, FCMSendRequest: %+v, err: %v",
			itemName, i.ID.Hex(), fcmReq, err,
		)
		return
	}
	s.Logger.Infof("notify: Send notification results for Item: %s, ID: %s, success: %d, failure: %d",
		itemName, i.ID.Hex(), fcmResp.Success, fcmResp.Failure)
	s.Logger.Debugf("notify: FCMSendResponse for Item: %s, ID: %s, resp: %+v", itemName, i.ID.Hex(), fcmResp)

	updatedUserCount, err := s.DB.UserTrackedItemNotificationCountIncrement(ctx, notifiedUserIDs, i.ID)
	if err != nil {
		s.Logger.Errorf("notify: Error incrementing User TrackedItem Notification Counts, err: %v", err)
		return
	}
	if updatedUserCount != len(notifiedUserIDs) {
		s.Logger.Errorf(
			"notify: Updated User count mismatch with notified UserIDs, updated: %d, notified: %d, notifiedUserIDs: %v for Item: %s, ID: %s",
			updatedUserCount, len(notifiedUserIDs), notifiedUserIDs, itemName, i.ID.Hex(),
		)
	}
}

func shouldNotify(ti model.TrackedItem, itemPrice int, itemStock int) bool {
	if ti.NotificationEnabled &&
		itemPrice <= ti.PriceLowerThreshold &&
		itemStock > 0 {
		return true
	}
	return false
}
