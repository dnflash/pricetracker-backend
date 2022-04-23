package server

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"net/http"
	"net/url"
	"pricetracker/internal/database"
	"time"
)

type siteType int

const (
	siteTypeInvalid siteType = iota
	siteShopee
	siteTokopedia
	siteBlibli
)

func siteTypeFromURL(urlStr string) (siteType, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return siteTypeInvalid, err
	}

	if parsedURL.Host == "shopee.co.id" {
		return siteShopee, nil
	} else if parsedURL.Host == "www.tokopedia.com" {
		return siteTokopedia, nil
	} else if parsedURL.Host == "www.blibli.com" {
		return siteBlibli, nil
	}

	return siteTypeInvalid, errors.New("invalid site url")
}

func (s Server) writeResponse(w http.ResponseWriter, response any) {
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.Logger.Errorf("Error encoding response: %+v, err: %+v", response, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s Server) itemAdd() http.HandlerFunc {
	type request struct {
		URL string `json:"url"`
	}
	type response struct {
		ItemID         string `json:"item_id"`
		Name           string `json:"name"`
		ProductID      string `json:"product_id"`
		ProductVariant string `json:"product_variant"`
		Price          int    `json:"price"`
		Stock          int    `json:"stock"`
		MerchantName   string `json:"merchant_name"`
		Site           string `json:"site"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		siteType, err := siteTypeFromURL(req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch siteType {
		case siteShopee:
			shopeeItem, err := s.Client.ShopeeGetItem(req.URL)
			if err != nil {
				s.Logger.Errorf("Error getting Shopee item, url: %+v, err: %+v", req.URL, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			item := database.Item{
				URL:            req.URL,
				Name:           shopeeItem.Name,
				ProductID:      shopeeItem.ItemID,
				ProductVariant: "-",
				MerchantName:   shopeeItem.ShopID,
				Site:           "Shopee",
				CreatedAt:      primitive.NewDateTimeFromTime(time.Now()),
				UpdatedAt:      primitive.NewDateTimeFromTime(time.Now()),
			}

			id, err := s.DB.ItemInsert(r.Context(), item)
			if err != nil {
				s.Logger.Errorf("Error inserting Item: %+v, err: %+v", item, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			objID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				s.Logger.Errorf("Error creating ObjectID from hex string: %+v, err: %+v", id, err)
			}

			itemHistory := database.ItemHistory{
				ItemID:    objID,
				Price:     shopeeItem.Price,
				Stock:     shopeeItem.Stock,
				Timestamp: primitive.NewDateTimeFromTime(time.Now()),
			}
			if err = s.DB.ItemHistoryInsert(r.Context(), itemHistory); err != nil {
				s.Logger.Errorf("Error inserting ItemHistory: %+v, err: %+v", itemHistory, err)
			}

			resp := response{
				ItemID:         id,
				Name:           item.Name,
				ProductID:      item.ProductID,
				ProductVariant: item.ProductVariant,
				Price:          itemHistory.Price,
				Stock:          itemHistory.Stock,
				MerchantName:   item.MerchantName,
				Site:           item.Site,
			}

			s.writeResponse(w, resp)
		}
	}
}

func (s Server) itemCheck() http.HandlerFunc {
	type request struct {
		URL string `json:"url"`
	}
	type response struct {
		Name           string `json:"name"`
		ProductID      string `json:"product_id"`
		ProductVariant string `json:"product_variant"`
		Price          int    `json:"price"`
		Stock          int    `json:"stock"`
		MerchantName   string `json:"merchant_name"`
		Site           string `json:"site"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		siteType, err := siteTypeFromURL(req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch siteType {
		case siteShopee:
			shopeeItem, err := s.Client.ShopeeGetItem(req.URL)
			if err != nil {
				s.Logger.Errorf("Error getting Shopee item, url: %+v, err: %+v", req.URL, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			resp := response{
				Name:           shopeeItem.Name,
				ProductID:      shopeeItem.ItemID,
				ProductVariant: "-",
				Price:          shopeeItem.Price,
				Stock:          shopeeItem.Stock,
				MerchantName:   shopeeItem.ShopID,
				Site:           "Shopee",
			}

			s.writeResponse(w, resp)
		}
	}
}

func (s Server) itemGetOne() http.HandlerFunc {
	type response struct {
		ItemID         string `json:"item_id"`
		Name           string `json:"name"`
		ProductID      string `json:"product_id"`
		ProductVariant string `json:"product_variant"`
		Price          int    `json:"price"`
		Stock          int    `json:"stock"`
		MerchantName   string `json:"merchant_name"`
		Site           string `json:"site"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["itemID"]
		item, err := s.DB.ItemFind(r.Context(), id)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, "item not found", http.StatusNotFound)
				return
			} else {
				s.Logger.Errorf("Error finding Item from ID: %+v, err: %+v", id, err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		itemHist, err := s.DB.ItemHistoryFindLatest(r.Context(), item.ID)
		if err != nil {
			s.Logger.Errorf("Error finding ItemHistory from ItemID: %+v, err: %+v", item.ID, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := response{
			ItemID:         id,
			Name:           item.Name,
			ProductID:      item.ProductID,
			ProductVariant: item.ProductVariant,
			Price:          itemHist.Price,
			Stock:          itemHist.Stock,
			MerchantName:   item.MerchantName,
			Site:           item.Site,
		}

		s.writeResponse(w, resp)
	}
}

func (s Server) itemGetAll() http.HandlerFunc {
	type item struct {
		ItemID         string `json:"item_id"`
		Name           string `json:"name"`
		ProductID      string `json:"product_id"`
		ProductVariant string `json:"product_variant"`
		Price          int    `json:"price"`
		Stock          int    `json:"stock"`
		MerchantName   string `json:"merchant_name"`
		Site           string `json:"site"`
	}
	type response []item

	return func(w http.ResponseWriter, r *http.Request) {
		is, err := s.DB.ItemFindAll(r.Context())
		if err != nil {
			s.Logger.Errorf("Error getting all Item, err: %+v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := response{}
		for _, i := range is {
			ih, err := s.DB.ItemHistoryFindLatest(r.Context(), i.ID)
			if err != nil {
				s.Logger.Errorf("Error getting ItemHistory for Item: %+v, err: %+v", i, err)
				continue
			}
			resp = append(resp, item{
				ItemID:         i.ID.Hex(),
				Name:           i.Name,
				ProductID:      i.ProductID,
				ProductVariant: i.ProductVariant,
				Price:          ih.Price,
				Stock:          ih.Stock,
				MerchantName:   i.MerchantName,
				Site:           i.Site,
			})
		}

		s.writeResponse(w, resp)
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
			s.Logger.Errorf("Error creating ObjectID from hex string: %+v, err: %+v", id, err)
		}

		ihs, err := s.DB.ItemHistoryFindRange(r.Context(), objID, req.Start, req.End)
		if err != nil {
			s.Logger.Errorf("Error getting ItemHistories, err: %+v", err)
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

		s.writeResponse(w, resp)
	}
}
