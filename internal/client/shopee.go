package client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v9"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"net/url"
	"pricetracker/internal/misc"
	"pricetracker/internal/model"
	"strconv"
	"strings"
	"time"
)

var ErrShopee = errors.New("Shopee error")
var ErrShopeeItemNotFound = errors.New("Shopee item not found")

type shopeeItemResponse struct {
	Error      int         `json:"error"`
	Data       *shopeeItem `json:"data"`
	ActionType int         `json:"action_type"`
}

type shopeeItem struct {
	ShopID         int              `json:"shopid"`
	ItemID         int              `json:"itemid"`
	Name           string           `json:"name"`
	Price          int              `json:"price"`
	Stock          int              `json:"stock"`
	Image          string           `json:"image"`
	Description    string           `json:"description"`
	HistoricalSold int              `json:"historical_sold"`
	ItemRating     shopeeItemRating `json:"item_rating"`
}

type shopeeItemRating struct {
	RatingStar float64 `json:"rating_star"`
}

type shopeeSearchResponse struct {
	NoMore bool               `json:"nomore"`
	Items  []shopeeSearchItem `json:"items"`
}

type shopeeSearchItem struct {
	ItemBasic shopeeItem `json:"item_basic"`
	AdsID     int        `json:"adsid"`
}

func (c Client) ShopeeGetItem(url string, useCache bool) (model.Item, error) {
	ctx := context.TODO()
	var i model.Item
	shopID, itemID, ok := shopeeGetShopAndItemID(url)
	if !ok {
		return i, errors.Wrapf(ErrShopeeItemNotFound, "error getting ShopID and ItemID from URL: %s", url)
	}
	apiURL := fmt.Sprintf("https://shopee.co.id/api/v4/item/get?shopid=%s&itemid=%s", shopID, itemID)
	cacheKey := "SGI-" + apiURL
	if useCache {
		cached, err := c.Redis.Get(ctx, cacheKey).Result()
		if err == nil {
			c.Logger.Infof("ShopeeGetItem: Cache found, key: %s", cacheKey)
			if err = json.Unmarshal([]byte(cached), &i); err == nil {
				return i, nil
			} else {
				c.Logger.Errorf("ShopeeGetItem: Error unmarshalling cache, key: %s, err: %v", cacheKey, err)
			}
		} else {
			if err != redis.Nil {
				c.Logger.Errorf("ShopeeGetItem: Error getting getting Redis cache with key: %s, err: %v", cacheKey, err)
			}
		}
	}
	req, err := shopeeNewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return i, err
	}
	c.Logger.Infof("ShopeeGetItem: Sending request to %s", apiURL)
	resp, err := c.ShopeeClient.Do(req)
	if err != nil {
		return i, errors.Wrapf(ErrShopee, "error doing request:\n%#v,\nerr: %v", req, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Errorf("ShopeeGetItem: Error closing response body, resp:\n%#v,\nreq:\n%#v,\nerr: %v", resp, req, err)
		}
	}()

	shopeeItemResp := shopeeItemResponse{}
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 300000))
	if err != nil {
		return i, errors.Wrapf(err, "error reading ShopeeItemAPI response body, status: %s, body:\n%s,\nreq:\n%#v", resp.Status, body, req)
	}
	if err = json.Unmarshal(body, &shopeeItemResp); err != nil {
		return i, errors.Wrapf(err,
			"error unmarshalling ShopeeItemAPI response body, status: %s, body:\n%s,\nreq:\n%#v", resp.Status, body, req)
	}

	if shopeeItemResp.Error == 4 {
		return i, errors.Wrapf(ErrShopeeItemNotFound, "Shopee item not found, status: %s, body:\n%s,\nreq:\n%#v", resp.Status, body, req)
	}
	if shopeeItemResp.ActionType != 0 || shopeeItemResp.Data == nil {
		return i, errors.Wrapf(ErrShopee, "error getting data from ShopeeItemAPI, status: %s, body:\n%s,\nreq:\n%#v", resp.Status, body, req)
	}

	i = shopeeItemResp.Data.toItem()
	if iJSON, err := json.Marshal(i); err != nil {
		c.Logger.Errorf("ShopeeGetItem: Error marshalling Items to cache, key: %s, Item: %+v, err: %v", cacheKey, i, err)
	} else {
		if err = c.Redis.Set(ctx, cacheKey, iJSON, 1*time.Hour).Err(); err != nil {
			c.Logger.Errorf("ShopeeGetItem: Error caching Items, key: %s, Item: %+v, err: %v", cacheKey, i, err)
		}
	}

	return i, nil
}

func shopeeGetShopAndItemID(urlStr string) (shopID string, itemID string, ok bool) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", false
	}
	if strings.HasPrefix(parsedURL.Path, "/product/") {
		if sp := strings.Split(parsedURL.Path, "/"); len(sp) >= 4 {
			return sp[2], sp[3], true
		}
		return "", "", false
	}
	if sp := strings.Split(parsedURL.Path, "."); len(sp) >= 3 {
		return sp[len(sp)-2], sp[len(sp)-1], true
	}
	return "", "", false
}

func (c Client) ShopeeSearch(query string) ([]model.Item, error) {
	ctx := context.TODO()
	var is []model.Item
	apiURL := "https://shopee.co.id/api/v4/search/search_items"

	cacheKey := "SS-" + query
	cached, err := c.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		c.Logger.Infof("ShopeeSearch: Cache found, key: %s", cacheKey)
		if err = json.Unmarshal([]byte(cached), &is); err == nil {
			return is, nil
		} else {
			c.Logger.Errorf("ShopeeSearch: Error unmarshalling cache, key: %s, err: %v", cacheKey, err)
		}
	} else {
		if err != redis.Nil {
			c.Logger.Errorf("ShopeeSearch: Error getting getting Redis cache with key: %s, err: %v", cacheKey, err)
		}
	}

	req, err := shopeeNewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return is, err
	}
	qp := url.Values{
		"by":        []string{"relevancy"},
		"keyword":   []string{query},
		"limit":     []string{"10"},
		"newest":    []string{"0"},
		"order":     []string{"desc"},
		"page_type": []string{"search"},
		"scenario":  []string{"PAGE_GLOBAL_SEARCH"},
		"version":   []string{"2"},
	}.Encode()
	req.URL.RawQuery = strings.ReplaceAll(qp, "+", "%20")

	c.Logger.Infof("ShopeeSearch: Sending request to %s", apiURL)
	resp, err := c.ShopeeClient.Do(req)
	if err != nil {
		return is, errors.Wrapf(ErrShopee, "error doing request:\n%#v,\nerr: %v", req, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Errorf("ShopeeSearch: Error closing response body, resp:\n%#v,\nreq:\n%#v,\nerr: %v", resp, req, err)
		}
	}()

	shopeeSearchResp := shopeeSearchResponse{}
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 300000))
	if err != nil {
		return is, errors.Wrapf(err, "error reading ShopeeSearchAPI response body, status: %s, body:\n%s,\nreq:\n%#v", resp.Status, body, req)
	}
	if err = json.Unmarshal(body, &shopeeSearchResp); err != nil {
		return is, errors.Wrapf(err,
			"error unmarshalling ShopeeSearchAPI response body, status: %s, body:\n%s,\nreq:\n%#v", resp.Status, body, req)
	}

	if len(shopeeSearchResp.Items) == 0 && !shopeeSearchResp.NoMore {
		return is, errors.Wrapf(ErrShopee, "error getting data from ShopeeSearchAPI, status: %s, body:\n%s,\nreq:\n%#v", resp.Status, body, req)
	}

	for _, searchItem := range shopeeSearchResp.Items {
		if searchItem.AdsID != 0 {
			continue
		}
		is = append(is, searchItem.ItemBasic.toItem())
	}

	if isJSON, err := json.Marshal(is); err != nil {
		c.Logger.Errorf("ShopeeSearch: Error marshalling Item to cache, key: %s, Item: %+v, err: %v", cacheKey, is, err)
	} else {
		if err = c.Redis.Set(ctx, cacheKey, isJSON, 12*time.Hour).Err(); err != nil {
			c.Logger.Errorf("ShopeeSearch: Error caching Item, key: %s, Item: %+v, err: %v", cacheKey, is, err)
		}
	}

	return is, nil
}

func (si shopeeItem) toItem() model.Item {
	return model.Item{
		Site:        "Shopee",
		MerchantID:  strconv.Itoa(si.ShopID),
		ProductID:   strconv.Itoa(si.ItemID),
		URL:         fmt.Sprintf("https://shopee.co.id/product/%d/%d", si.ShopID, si.ItemID),
		Name:        si.Name,
		Price:       si.Price / 100000,
		Stock:       si.Stock,
		ImageURL:    "https://cf.shopee.co.id/file/" + si.Image,
		Description: misc.StringLimit(si.Description, 2500),
		Rating:      si.ItemRating.RatingStar,
		Sold:        si.HistoricalSold,
	}
}

func shopeeNewRequest(method string, url string, body io.Reader) (*http.Request, error) {
	req, err := newRequest(method, url, body)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating request from URL: %s", url)
	}
	//req.AddCookie(&http.Cookie{
	//	Name:  "SPC_U",
	//	Value: "-",
	//})
	//req.AddCookie(&http.Cookie{
	//	Name:  "SPC_F",
	//	Value: "-",
	//})
	return req, nil
}
