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
	"pricetracker/internal/misc"
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
	if parsedURL.Host == "" {
		parsedURL, err = url.Parse("https://" + urlStr)
		if err != nil {
			return siteTypeInvalid, "", err
		}
	}
	cleanURL := "https://" + parsedURL.Host + parsedURL.Path
	if parsedURL.Host == "shopee.co.id" {
		return siteShopee, cleanURL, nil
	} else if parsedURL.Host == "www.tokopedia.com" || parsedURL.Host == "tokopedia.com" || parsedURL.Host == "tokopedia.link" {
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
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Debugf("itemAdd: Error decoding JSON, err: %v", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		urlSiteType, cleanURL, err := siteTypeAndCleanURL(req.URL)
		if err != nil {
			s.Logger.Debugf("itemAdd: Bad url: %s, err: %v", req.URL, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var ecommerceItem model.Item
		switch urlSiteType {
		case siteShopee:
			ecommerceItem, err = s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrShopee) {
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
		case siteTokopedia:
			ecommerceItem, err = s.Client.TokopediaGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrTokopedia) {
					s.Logger.Errorf("itemAdd: Error getting Tokopedia item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				} else if errors.Is(err, client.ErrTokopediaItemNotFound) {
					s.Logger.Debugf("itemAdd: Item not found when getting Tokopedia item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				} else {
					s.Logger.Errorf("itemAdd: Error getting Tokopedia item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
			}
		case siteBlibli:
			ecommerceItem, err = s.Client.BlibliGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrBlibli) {
					s.Logger.Errorf("itemAdd: Error getting Blibli item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				} else if errors.Is(err, client.ErrBlibliItemNotFound) {
					s.Logger.Debugf("itemAdd: Item not found when getting Blibli item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				} else {
					s.Logger.Errorf("itemAdd: Error getting Blibli item with url: %s, err: %v", cleanURL, err)
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
					Price:     ecommerceItem.Price,
					Stock:     ecommerceItem.Stock,
					Rating:    ecommerceItem.Rating,
					Sold:      ecommerceItem.Sold,
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

		urlSiteType, cleanURL, err := siteTypeAndCleanURL(req.URL)
		if err != nil {
			s.Logger.Debugf("itemCheck: Bad url: %s, err: %v", req.URL, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var ecommerceItem model.Item
		switch urlSiteType {
		case siteShopee:
			ecommerceItem, err = s.Client.ShopeeGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrShopee) {
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
		case siteTokopedia:
			ecommerceItem, err = s.Client.TokopediaGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrTokopedia) {
					s.Logger.Errorf("itemCheck: Error getting Tokopedia item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				} else if errors.Is(err, client.ErrTokopediaItemNotFound) {
					s.Logger.Debugf("itemCheck: Item not found when getting Tokopedia item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				} else {
					s.Logger.Errorf("itemCheck: Error getting Tokopedia item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
			}
		case siteBlibli:
			ecommerceItem, err = s.Client.BlibliGetItem(cleanURL)
			if err != nil {
				if errors.Is(err, client.ErrBlibli) {
					s.Logger.Errorf("itemCheck: Error getting Blibli item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
					return
				} else if errors.Is(err, client.ErrBlibliItemNotFound) {
					s.Logger.Debugf("itemCheck: Item not found when getting Blibli item with url: %s, err: %v", cleanURL, err)
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				} else {
					s.Logger.Errorf("itemCheck: Error getting Blibli item with url: %s, err: %v", cleanURL, err)
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
		tid := getTraceContext(r.Context()).traceID
		var bc string
		var qa [2]string
		qa[0] = r.URL.Query().Get("query")
		if qa[0] != "" {
			qa[0] = qa[0][:misc.Min(len(qa[0]), 100)]
			cleanedQuery := misc.CleanString(qa[0])
			if qa[0] != cleanedQuery {
				s.Logger.Debugf("itemSearch: Cleaned search query, original: %#v, cleaned: %#v, TraceID: %s",
					qa[0], cleanedQuery, tid)
				qa[0] = cleanedQuery
			}
		}
		if qa[0] == "" {
			if bc = r.URL.Query().Get("bc"); bc == "" {
				s.Logger.Debugf("itemSearch: No search parameters supplied, TraceID: %s", tid)
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			} else {
				b, err := s.DB.BarcodeFind(r.Context(), bc)
				if err != nil {
					if errors.Is(err, mongo.ErrNoDocuments) {
						s.Logger.Debugf("itemSearch: Barcode %#v not found, TraceID: %s", bc, tid)
						s.writeJsonResponse(w, response([]model.Item{}), http.StatusOK)
						return
					} else {
						s.Logger.Errorf("itemSearch: Error finding barcode %#v, err: %v, TraceID: %s", bc, err, tid)
						http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
						return
					}
				}
				qa[0] = b.Query1
				qa[1] = b.Query2
				if qa[0] == qa[1] {
					qa[1] = ""
				}
				s.Logger.Infof("itemSearch: Barcode %#v found, q1: %#v, q2: %#v, TraceID: %s", bc, qa[0], qa[1], tid)
			}
		} else {
			s.Logger.Infof("itemSearch: Searching items with query: %#v, TraceID: %s", qa[0], tid)
		}
		var shopeeItems []model.Item
		var tokopediaItems []model.Item
		for i, q := range qa {
			if q != "" {
				if len(shopeeItems) < 3 {
					is, err := s.Client.ShopeeSearch(q)
					if err == nil {
						if len(is) > 0 && len(shopeeItems) > 0 {
							shopeeItems = mergeItemSlices(shopeeItems, is)
						} else if len(shopeeItems) == 0 {
							shopeeItems = is
						}
						s.Logger.Debugf("itemSearch: Searched Shopee with q%d: %#v, %d item(s) found, TraceID: %s", i+1, q, len(is), tid)
					} else {
						s.Logger.Errorf("itemSearch: Error searching Shopee with q%d: %#v, err: %v, TraceID: %s", i+1, q, err, tid)
					}
				}
				if len(tokopediaItems) < 3 {
					is, err := s.Client.TokopediaSearch(q)
					if err == nil {
						if len(is) > 0 && len(tokopediaItems) > 0 {
							tokopediaItems = mergeItemSlices(tokopediaItems, is)
						} else if len(tokopediaItems) == 0 {
							tokopediaItems = is
						}
						s.Logger.Debugf("itemSearch: Searched Tokopedia with q%d: %#v, %d item(s) found, TraceID: %s", i+1, q, len(is), tid)
					} else {
						s.Logger.Errorf("itemSearch: Error searching Tokopedia with q%d: %#v, err: %v, TraceID: %s", i+1, q, err, tid)
					}
				}
			} else if bc != "" {
				s.Logger.Debugf("itemSearch: Barcode %#v q%d is empty, TraceID: %s", bc, i+1, tid)
			}
		}
		shopeeItems = shopeeItems[:misc.Min(len(shopeeItems), 3)]
		tokopediaItems = tokopediaItems[:misc.Min(len(tokopediaItems), 3)]
		items := make([]model.Item, 0, len(shopeeItems)+len(tokopediaItems))
		items = append(items, shopeeItems...)
		items = append(items, tokopediaItems...)
		s.writeJsonResponse(w, response(items), http.StatusOK)
	}
}

func mergeItemSlices(is []model.Item, is2 []model.Item) []model.Item {
	deduplicated := make([]model.Item, 0, len(is2))
	for _, v := range is2 {
		var duplicated bool
		for _, v2 := range is {
			if v2.Site == v.Site && v2.ProductID == v.ProductID {
				duplicated = true
				break
			}
		}
		if !duplicated {
			deduplicated = append(deduplicated, v)
		}
	}
	return append(is, deduplicated...)
}
