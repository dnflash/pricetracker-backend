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
			http.Error(w, err.Error(), http.StatusBadRequest)
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
				if errors.Is(err, client.ShopeeItemNotFoundErr) {
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
			http.Error(w, err.Error(), http.StatusBadRequest)
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
				if errors.Is(err, client.ShopeeItemNotFoundErr) {
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
		database.Item
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["itemID"]
		if id == "" {
			s.Logger.Debugf("itemGetOne: itemID not supplied")
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		i, err := s.DB.ItemFindOne(r.Context(), id)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				s.Logger.Debugf("itemGetOne: No documents found for Item with ID: %s, err: %v", id, err)
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			} else {
				s.Logger.Errorf("itemGetOne: Error finding Item with ID: %s, err: %v", id, err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}

		s.writeJsonResponse(w, response{
			ItemID: i.ID.Hex(),
			Item:   i,
		})
	}
}

func (s Server) itemGetAll() http.HandlerFunc {
	type item struct {
		ItemID string `json:"item_id"`
		database.Item
	}
	type response []item
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

		itemIDs := []primitive.ObjectID{}
		for _, ti := range u.TrackedItems {
			itemIDs = append(itemIDs, ti.ItemID)
		}

		is, err := s.DB.ItemsFind(r.Context(), itemIDs)
		if err != nil {
			s.Logger.Errorf("itemGetAll: Error getting all Item for User with ID: %s, err: %v", uc.id, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		resp := response{}
		for _, i := range is {
			resp = append(resp, item{
				ItemID: i.ID.Hex(),
				Item:   i,
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

	type itemHistory struct {
		Price     int       `json:"pr"`
		Stock     int       `json:"st"`
		Timestamp time.Time `json:"ts"`
	}
	type response []itemHistory

	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		id := mux.Vars(r)["itemID"]
		objID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			s.Logger.Errorf("Error creating ObjectID from hex string: %s, err: %v", id, err)
		}

		ihs, err := s.DB.ItemHistoryFindRange(r.Context(), objID, req.Start, req.End)
		if err != nil {
			s.Logger.Errorf("Error getting ItemHistories, err: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := response{}
		for _, ih := range ihs {
			resp = append(resp, itemHistory{
				Price:     ih.Price,
				Stock:     ih.Stock,
				Timestamp: ih.Timestamp.Time().UTC(),
			})
		}

		s.writeJsonResponse(w, resp)
	}
}
