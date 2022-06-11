package server

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"net/http"
	"net/url"
	"pricetracker/internal/client"
	"pricetracker/internal/model"
	"time"
)

type siteType int

const (
	siteTypeInvalid siteType = iota
	siteShopee
	siteTokopedia
	siteBlibli
)

func siteTypeAndCleanURL(urlStr string) (siteType, string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return siteTypeInvalid, "", err
	}
	cleanURL := "https://" + parsedURL.Host + parsedURL.Path
	if parsedURL.Host == "shopee.co.id" {
		return siteShopee, cleanURL, nil
	} else if parsedURL.Host == "www.tokopedia.com" {
		return siteTokopedia, cleanURL, nil
	} else if parsedURL.Host == "www.blibli.com" {
		return siteBlibli, cleanURL, nil
	}
	return siteTypeInvalid, "", errors.Errorf("invalid site url: %s", cleanURL)
}

func (s Server) itemAdd() http.HandlerFunc {
	type request struct {
		URL                 string `json:"url"`
		PriceLowerThreshold int    `json:"price_lower_threshold"`
		NotificationEnabled bool   `json:"notification_enabled"`
	}
	type response struct {
		ItemID string `json:"item_id"`
		model.TrackedItem
		Item model.Item `json:"item"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, err := getUserContext(r.Context())
		if err != nil {
			s.Logger.Errorf("itemAdd: Error getting userContext, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		req := request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("itemAdd: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		siteType, cleanURL, err := siteTypeAndCleanURL(req.URL)
		if err != nil {
			s.Logger.Debugf("itemAdd: Bad url: %s, err: %v", req.URL, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var ecommerceItem model.Item
		switch siteType {
		case siteShopee:
			ecommerceItem, err = s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrShopeeItem) {
					s.Logger.Errorf("itemAdd: Error getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				} else if errors.Is(err, client.ErrShopeeItemNotFound) {
					s.Logger.Debugf("itemAdd: Item not found when getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				} else {
					s.Logger.Errorf("itemAdd: Error getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
			}
		}
		i, err := s.DB.ItemFindExisting(r.Context(), ecommerceItem)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				i = ecommerceItem
				i.PriceHistoryHighest = i.Price
				i.PriceHistoryLowest = i.Price
				itemID, err := s.DB.ItemInsert(r.Context(), i)
				if err != nil {
					s.Logger.Errorf("itemAdd: Error inserting Item, err: %v", err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				i.ID, err = primitive.ObjectIDFromHex(itemID)
				if err != nil {
					s.Logger.Errorf("itemAdd: Error creating ObjectID from hex: %s, err: %v", itemID, err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				ih := model.ItemHistory{
					ItemID:    i.ID,
					Price:     i.Price,
					Stock:     i.Stock,
					Timestamp: primitive.NewDateTimeFromTime(time.Now()),
				}
				if err = s.DB.ItemHistoryInsert(r.Context(), ih); err != nil {
					s.Logger.Errorf("itemAdd: Error inserting ItemHistory, err: %v", err)
				}
			} else {
				s.Logger.Errorf("itemAdd: Error finding existing Item, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		} else {
			i.UpdateWith(ecommerceItem)
			if err = s.DB.ItemUpdate(r.Context(), i); err != nil {
				s.Logger.Errorf("itemAdd: Error updating existing Item, err: %v", err)
			}
		}

		tracked := itemTracked(i.ID.Hex(), uc.user.TrackedItems)
		if len(uc.user.TrackedItems) >= 25 && !tracked {
			s.Logger.Debugf("itemAdd: Failed to add item, TrackedItems are limited to 25 for each User, UserID: %s, ItemID: %s",
				uc.user.ID.Hex(), i.ID.Hex())
			http.Error(w, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
			return
		}
		ti := model.TrackedItem{
			ItemID:              i.ID,
			PriceInitial:        i.Price,
			PriceLowerThreshold: req.PriceLowerThreshold,
			NotificationCount:   0,
			NotificationEnabled: req.NotificationEnabled,
		}
		if tracked {
			if err = s.DB.UserTrackedItemUpdate(r.Context(), uc.user.ID.Hex(), ti); err != nil {
				s.Logger.Errorf("itemAdd: Error updating TrackedItem on User, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		} else {
			if err = s.DB.UserTrackedItemAdd(r.Context(), uc.user.ID.Hex(), ti); err != nil {
				s.Logger.Errorf("itemAdd: Error adding TrackedItem to User, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		s.writeJsonResponse(w, response{
			ItemID:      i.ID.Hex(),
			TrackedItem: ti,
			Item:        i,
		}, http.StatusOK)
	}
}

func (s Server) itemCheck() http.HandlerFunc {
	type request struct {
		URL string `json:"url"`
	}
	type response model.Item
	return func(w http.ResponseWriter, r *http.Request) {
		req := request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("itemCheck: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		siteType, cleanURL, err := siteTypeAndCleanURL(req.URL)
		if err != nil {
			s.Logger.Debugf("itemCheck: Bad url: %s, err: %v", req.URL, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var ecommerceItem model.Item
		switch siteType {
		case siteShopee:
			ecommerceItem, err = s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrShopeeItem) {
					s.Logger.Errorf("itemCheck: Error getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				} else if errors.Is(err, client.ErrShopeeItemNotFound) {
					s.Logger.Debugf("itemCheck: Item not found when getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				} else {
					s.Logger.Errorf("itemCheck: Error getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
			}
		}
		i, err := s.DB.ItemFindExisting(r.Context(), ecommerceItem)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				i = ecommerceItem
				i.PriceHistoryHighest = i.Price
				i.PriceHistoryLowest = i.Price
			} else {
				s.Logger.Errorf("itemCheck: Error finding existing Item, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		} else {
			i.UpdateWith(ecommerceItem)
			if err = s.DB.ItemUpdate(r.Context(), i); err != nil {
				s.Logger.Errorf("itemCheck: Error updating existing Item, err: %v", err)
			}
		}
		s.writeJsonResponse(w, response(i), http.StatusOK)
	}
}

func (s Server) itemUpdate() http.HandlerFunc {
	type request struct {
		ItemID              string `json:"item_id"`
		PriceLowerThreshold int    `json:"price_lower_threshold"`
		NotificationEnabled bool   `json:"notification_enabled"`
	}
	type response struct {
		Success bool `json:"success"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, err := getUserContext(r.Context())
		if err != nil {
			s.Logger.Errorf("itemUpdate: Error getting userContext, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		req := request{}
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("itemUpdate: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		if !itemTracked(req.ItemID, uc.user.TrackedItems) {
			s.Logger.Debugf("itemUpdate: Item not tracked on User with ID: %s, ItemID: %s", uc.user.ID.Hex(), req.ItemID)
			s.writeJsonResponse(w, response{Success: false}, http.StatusUnprocessableEntity)
			return
		}

		itemOID, err := primitive.ObjectIDFromHex(req.ItemID)
		if err != nil {
			s.Logger.Debugf("itemUpdate: error generating ObjectID from hex: %s, err: %v", req.ItemID, err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		ti := model.TrackedItem{
			ItemID:              itemOID,
			PriceLowerThreshold: req.PriceLowerThreshold,
			NotificationEnabled: req.NotificationEnabled,
			NotificationCount:   0,
		}
		if err = s.DB.UserTrackedItemUpdate(r.Context(), uc.user.ID.Hex(), ti); err != nil {
			s.Logger.Errorf("itemUpdate: Error updating TrackedItem for User with ID: %s, TrackedItem: %+v, err: %v", uc.user.ID.Hex(), ti, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		s.writeJsonResponse(w, response{Success: true}, http.StatusOK)
	}
}

func (s Server) itemRemove() http.HandlerFunc {
	type request struct {
		ItemID string `json:"item_id"`
	}
	type response struct {
		Success bool `json:"success"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, err := getUserContext(r.Context())
		if err != nil {
			s.Logger.Errorf("itemRemove: Error getting userContext, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		req := request{}
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("itemRemove: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		if !itemTracked(req.ItemID, uc.user.TrackedItems) {
			s.Logger.Debugf("itemRemove: Item not tracked on User with ID: %s, ItemID: %s", uc.user.ID.Hex(), req.ItemID)
			s.writeJsonResponse(w, response{Success: false}, http.StatusUnprocessableEntity)
			return
		}
		if err = s.DB.UserTrackedItemRemove(r.Context(), uc.user.ID.Hex(), req.ItemID); err != nil {
			s.Logger.Errorf("itemRemove: Error removing TrackedItem from User with ID: %s, ItemID: %s, err: %v", uc.user.ID.Hex(), req.ItemID, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		s.writeJsonResponse(w, response{Success: true}, http.StatusOK)
	}
}

func itemTracked(itemID string, tis []model.TrackedItem) bool {
	itemOID, err := primitive.ObjectIDFromHex(itemID)
	if err != nil {
		return false
	}
	for _, ti := range tis {
		if ti.ItemID == itemOID {
			return true
		}
	}
	return false
}

func (s Server) itemGetOne() http.HandlerFunc {
	type response struct {
		ItemID string `json:"item_id"`
		model.TrackedItem
		Item model.Item `json:"item"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, err := getUserContext(r.Context())
		if err != nil {
			s.Logger.Errorf("itemGetOne: Error getting userContext, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		itemID := mux.Vars(r)["itemID"]
		if itemID == "" {
			s.Logger.Debugf("itemGetOne: itemID not supplied")
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		i, err := s.DB.ItemFindOne(r.Context(), itemID)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) || errors.Is(err, primitive.ErrInvalidHex) {
				s.Logger.Debugf("itemGetOne: No documents found for Item with ID: %s, err: %v", itemID, err)
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			} else {
				s.Logger.Errorf("itemGetOne: Error finding Item with ID: %s, err: %v", itemID, err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}

		resp := response{
			ItemID: i.ID.Hex(),
			Item:   i,
		}
		for _, ti := range uc.user.TrackedItems {
			if ti.ItemID == i.ID {
				resp.TrackedItem = ti
				break
			}
		}
		s.writeJsonResponse(w, resp, http.StatusOK)
	}
}

func (s Server) itemGetAll() http.HandlerFunc {
	type userItem struct {
		ItemID string `json:"item_id"`
		model.TrackedItem
		Item model.Item `json:"item"`
	}
	type response []userItem
	return func(w http.ResponseWriter, r *http.Request) {
		uc, err := getUserContext(r.Context())
		if err != nil {
			s.Logger.Errorf("itemGetAll: Error getting userContext, err: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		var itemIDs []primitive.ObjectID
		for _, ti := range uc.user.TrackedItems {
			itemIDs = append(itemIDs, ti.ItemID)
		}

		resp := response{}
		if len(itemIDs) == 0 {
			s.writeJsonResponse(w, resp, http.StatusOK)
			return
		}
		is, err := s.DB.ItemsFind(r.Context(), itemIDs)
		if err != nil {
			s.Logger.Errorf("itemGetAll: Error getting all Item for User with ID: %s, err: %v", uc.user.ID.Hex(), err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		for _, ti := range uc.user.TrackedItems {
			var item model.Item
			for _, i := range is {
				if i.ID == ti.ItemID {
					item = i
					break
				}
			}
			resp = append(resp, userItem{
				ItemID:      ti.ItemID.Hex(),
				TrackedItem: ti,
				Item:        item,
			})
		}
		s.writeJsonResponse(w, resp, http.StatusOK)
	}
}

func (s Server) itemHistory() http.HandlerFunc {
	type request struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	}
	type response []model.ItemHistory
	return func(w http.ResponseWriter, r *http.Request) {
		req := request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("itemHistory: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		itemID := mux.Vars(r)["itemID"]
		if itemID == "" {
			s.Logger.Debug("itemHistory: itemID not supplied")
			s.writeJsonResponse(w, response{}, http.StatusOK)
			return
		}
		ihs, err := s.DB.ItemHistoryFindRange(r.Context(), itemID, req.Start, req.End)
		if err != nil {
			if errors.Is(err, primitive.ErrInvalidHex) {
				s.Logger.Debugf("itemHistory: itemID invalid, err: %v", err)
				s.writeJsonResponse(w, response{}, http.StatusOK)
				return
			} else {
				s.Logger.Errorf("itemHistory: Error getting ItemHistories, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		if len(ihs) == 0 {
			s.Logger.Debugf("itemHistory: No ItemHistories found for ItemID: %s", itemID)
			s.writeJsonResponse(w, response{}, http.StatusOK)
			return
		}
		s.writeJsonResponse(w, response(ihs), http.StatusOK)
	}
}

func (s Server) itemSearch() http.HandlerFunc {
	type response []model.Item
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if q == "" {
			s.Logger.Debugf("itemSearch: \"query\" query parameter is not supplied")
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		is, err := s.Client.ShopeeSearch(q)
		if err != nil {
			s.Logger.Errorf("itemSearch: Error searching Shopee with query: %s, err: %v", q, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		is = append([]model.Item{}, is...)
		if len(is) > 3 {
			is = is[:3]
		}
		s.writeJsonResponse(w, response(is), http.StatusOK)
	}
}
