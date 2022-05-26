package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"net/http"
	"net/url"
	"pricetracker/internal/client"
	"pricetracker/internal/database"
	"strconv"
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
		PriceLowerBound     int    `json:"price_lower_bound"`
		NotificationEnabled bool   `json:"notification_enabled"`
	}
	type response struct {
		ItemID string `json:"item_id"`
		database.Item
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, ok := r.Context().Value(userContextKey{}).(userContext)
		if !ok {
			s.Logger.Error("itemAdd: Error getting userContext")
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debug("itemAdd: Error decoding JSON, err:", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		siteType, cleanURL, err := siteTypeAndCleanURL(req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch siteType {
		case siteShopee:
			shopeeItem, err := s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrShopeeItemNotFound) {
					s.Logger.Debugf("itemAdd: Item not found when getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				}

				s.Logger.Errorf("itemAdd: Error getting Shopee item with url: %s, err: %v", cleanURL, err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			i := database.Item{
				URL:            fmt.Sprintf("https://shopee.co.id/product/%d/%d", shopeeItem.ShopID, shopeeItem.ItemID),
				Name:           shopeeItem.Name,
				ProductID:      strconv.Itoa(shopeeItem.ItemID),
				ProductVariant: "-",
				Price:          shopeeItem.Price,
				Stock:          shopeeItem.Stock,
				ImageURL:       shopeeItem.ImageURL,
				MerchantName:   strconv.Itoa(shopeeItem.ShopID),
				Site:           "Shopee",
			}

			id, err := s.DB.ItemInsert(r.Context(), i)
			if err != nil {
				s.Logger.Errorf("itemAdd: Error inserting Item: %+v, err: %v", i, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			objID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				s.Logger.Errorf("itemAdd: Error creating ObjectID from hex string: %s, err: %v", id, err)
			}

			ih := database.ItemHistory{
				ItemID:    objID,
				Price:     shopeeItem.Price,
				Stock:     shopeeItem.Stock,
				Timestamp: primitive.NewDateTimeFromTime(time.Now()),
			}
			if err = s.DB.ItemHistoryInsert(r.Context(), ih); err != nil {
				s.Logger.Errorf("itemAdd: Error inserting ItemHistory: %+v, err: %v", ih, err)
			}

			ti := database.TrackedItem{
				ItemID:              objID,
				PriceLowerBound:     req.PriceLowerBound,
				NotificationCount:   0,
				NotificationEnabled: req.NotificationEnabled,
			}
			if err = s.DB.UserTrackedItemUpdateOrAdd(r.Context(), uc.id, ti); err != nil {
				s.Logger.Errorf("itemAdd: Error adding TrackedItem to User, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			s.writeJsonResponse(w, response{
				ItemID: id,
				Item:   i,
			})
		}
	}
}

func (s Server) itemCheck() http.HandlerFunc {
	type request struct {
		URL string `json:"url"`
	}
	type response struct {
		database.Item
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debug("itemCheck: Error decoding JSON, err:", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		siteType, cleanURL, err := siteTypeAndCleanURL(req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch siteType {
		case siteShopee:
			shopeeItem, err := s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrShopeeItemNotFound) {
					s.Logger.Debugf("itemCheck: Item not found when getting Shopee item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				}

				s.Logger.Errorf("itemCheck: Error getting Shopee item with url: %s, err: %v", cleanURL, err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			i := database.Item{
				URL:            fmt.Sprintf("https://shopee.co.id/product/%d/%d", shopeeItem.ShopID, shopeeItem.ItemID),
				Name:           shopeeItem.Name,
				ProductID:      strconv.Itoa(shopeeItem.ItemID),
				ProductVariant: "-",
				Price:          shopeeItem.Price,
				Stock:          shopeeItem.Stock,
				ImageURL:       shopeeItem.ImageURL,
				MerchantName:   strconv.Itoa(shopeeItem.ShopID),
				Site:           "Shopee",
			}
			s.writeJsonResponse(w, response{i})
		}
	}
}

func (s Server) itemGetOne() http.HandlerFunc {
	type response struct {
		ItemID string `json:"item_id"`
		database.TrackedItem
		Item database.Item `json:"item"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		uc, ok := r.Context().Value(userContextKey{}).(userContext)
		if !ok {
			s.Logger.Error("itemGetOne: Error getting userContext")
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		u, err := s.DB.UserFindByID(r.Context(), uc.id)
		if err != nil {
			s.Logger.Error("itemGetOne: Error finding User with ID: %s", uc.id)
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
		for _, ti := range u.TrackedItems {
			if ti.ItemID == i.ID {
				resp.TrackedItem = ti
				break
			}
		}
		s.writeJsonResponse(w, resp)
	}
}

func (s Server) itemGetAll() http.HandlerFunc {
	type userItem struct {
		ItemID string `json:"item_id"`
		database.TrackedItem
		Item database.Item `json:"item"`
	}
	type response []userItem
	return func(w http.ResponseWriter, r *http.Request) {
		uc, ok := r.Context().Value(userContextKey{}).(userContext)
		if !ok {
			s.Logger.Error("itemGetAll: Error getting userContext")
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		u, err := s.DB.UserFindByID(r.Context(), uc.id)
		if err != nil {
			s.Logger.Error("itemGetAll: Error finding User with ID: %s", uc.id)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		var itemIDs []primitive.ObjectID
		for _, ti := range u.TrackedItems {
			itemIDs = append(itemIDs, ti.ItemID)
		}

		resp := response{}
		if len(itemIDs) == 0 {
			s.writeJsonResponse(w, resp)
			return
		}
		is, err := s.DB.ItemsFind(r.Context(), itemIDs)
		if err != nil {
			s.Logger.Errorf("itemGetAll: Error getting all Item for User with ID: %s, err: %v", uc.id, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		for _, ti := range u.TrackedItems {
			var item database.Item
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
		s.writeJsonResponse(w, resp)
	}
}

func (s Server) itemHistory() http.HandlerFunc {
	type request struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	}
	type response []database.ItemHistory
	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debug("itemHistory: Error decoding JSON, err:", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		itemID := mux.Vars(r)["itemID"]
		if itemID == "" {
			s.Logger.Debugf("itemHistory: itemID not supplied")
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		ihs, err := s.DB.ItemHistoryFindRange(r.Context(), itemID, req.Start, req.End)
		if err != nil {
			if errors.Is(err, primitive.ErrInvalidHex) {
				s.Logger.Debugf("itemHistory: itemID invalid, err: %v", err)
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			} else {
				s.Logger.Errorf("itemHistory: Error getting ItemHistories, err: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		if len(ihs) == 0 {
			s.Logger.Debugf("itemHistory: No ItemHistories found for ItemID: %s", itemID)
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		s.writeJsonResponse(w, response(ihs))
	}
}
