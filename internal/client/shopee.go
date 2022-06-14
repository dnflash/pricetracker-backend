package client

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"net/url"
	"pricetracker/internal/model"
	"strconv"
	"strings"
)

var ErrShopeeItem = errors.New("failed getting Shopee item")
var ErrShopeeItemNotFound = errors.New("Shopee item not found")
var ErrShopeeSearch = errors.New("failed searching Shopee")

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

func (c Client) ShopeeGetItem(url string) (model.Item, error) {
	var i model.Item
	shopID, itemID, ok := shopeeGetShopAndItemID(url)
	if !ok {
		return i, errors.Wrapf(ErrShopeeItemNotFound, "error getting ShopID and ItemID from URL: %s", url)
	}
	apiURL := fmt.Sprintf("https://shopee.co.id/api/v4/item/get?shopid=%s&itemid=%s", shopID, itemID)

	req, err := newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return i, errors.Wrapf(err, "error creating request from apiURL: %s", apiURL)
	}
	req.AddCookie(&http.Cookie{
		Name:  "SPC_U",
		Value: "-",
	})
	req.AddCookie(&http.Cookie{
		Name:  "SPC_F",
		Value: "-",
	})
	resp, err := c.Client.Do(req)
	if err != nil {
		return i, errors.Wrapf(err, "error doing request: %+v", req)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Error("ShopeeGetItem: Error closing response body on request to url: %s, err: %v", req.URL, err)
		}
	}()

	shopeeItemResp := shopeeItemResponse{}
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 300000))
	if err != nil {
		return i, errors.Wrapf(err, "error reading ShopeeItemAPI response body, status: %s, body: %s", resp.Status, body)
	}
	if err = json.Unmarshal(body, &shopeeItemResp); err != nil {
		return i, errors.Wrapf(err,
			"error unmarshalling ShopeeItemAPI response body, status: %s, body: %s", resp.Status, body)
	}

	if shopeeItemResp.Error == 4 {
		return i, errors.Wrapf(ErrShopeeItemNotFound, "Shopee item not found, status: %s, body: %s", resp.Status, body)
	}
	if shopeeItemResp.ActionType != 0 || shopeeItemResp.Data == nil {
		return i, errors.Wrapf(ErrShopeeItem, "error getting data from ShopeeItemAPI, status: %s, body: %s", resp.Status, body)
	}

	return shopeeItemResp.Data.toItem(), nil
}

func shopeeGetShopAndItemID(urlStr string) (shopID string, itemID string, ok bool) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", false
	}
	if strings.HasPrefix(parsedURL.Path, "/product/") {
		if sp := strings.Split(parsedURL.Path, "/"); len(sp) >= 3 {
			return sp[1], sp[2], true
		}
		return "", "", false
	}
	if sp := strings.Split(parsedURL.Path, "."); len(sp) >= 3 {
		return sp[1], sp[2], true
	}
	return "", "", false
}

func (c Client) ShopeeSearch(query string) ([]model.Item, error) {
	var is []model.Item
	apiURL := "https://shopee.co.id/api/v4/search/search_items"
	req, err := newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return is, errors.Wrapf(err, "error creating request from apiURL: %s", apiURL)
	}
	qp := url.Values{
		"by":        []string{"relevancy"},
		"keyword":   []string{query},
		"limit":     []string{"5"},
		"newest":    []string{"0"},
		"order":     []string{"desc"},
		"page_type": []string{"search"},
		"scenario":  []string{"PAGE_GLOBAL_SEARCH"},
		"version":   []string{"2"},
	}.Encode()
	req.URL.RawQuery = strings.ReplaceAll(qp, "+", "%20")
	req.AddCookie(&http.Cookie{
		Name:  "SPC_U",
		Value: "-",
	})
	req.AddCookie(&http.Cookie{
		Name:  "SPC_F",
		Value: "-",
	})
	resp, err := c.Client.Do(req)
	if err != nil {
		return is, errors.Wrapf(err, "error doing request: %+v", req)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Error("ShopeeSearch: Error closing response body on request to url: %s, err: %v", req.URL, err)
		}
	}()

	shopeeSearchResp := shopeeSearchResponse{}
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 300000))
	if err != nil {
		return is, errors.Wrapf(err, "error reading ShopeeSearchAPI response body, status: %s, body: %s", resp.Status, body)
	}
	if err = json.Unmarshal(body, &shopeeSearchResp); err != nil {
		return is, errors.Wrapf(err,
			"error unmarshalling ShopeeSearchAPI response body, status: %s, body: %s", resp.Status, body)
	}

	if len(shopeeSearchResp.Items) == 0 && !shopeeSearchResp.NoMore {
		return is, errors.Wrapf(ErrShopeeSearch, "error getting data from ShopeeSearchAPI, status: %s, body: %s", resp.Status, body)
	}

	for _, searchItem := range shopeeSearchResp.Items {
		if searchItem.AdsID != 0 {
			continue
		}
		is = append(is, searchItem.ItemBasic.toItem())
	}
	return is, nil
}

func (si shopeeItem) toItem() model.Item {
	if len(si.Description) > 2000 {
		si.Description = si.Description[:2000] + "..."
	}
	return model.Item{
		Site:        "Shopee",
		MerchantID:  strconv.Itoa(si.ShopID),
		ProductID:   strconv.Itoa(si.ItemID),
		URL:         fmt.Sprintf("https://shopee.co.id/product/%d/%d", si.ShopID, si.ItemID),
		Name:        si.Name,
		Price:       si.Price / 100000,
		Stock:       si.Stock,
		ImageURL:    "https://cf.shopee.co.id/file/" + si.Image,
		Description: si.Description,
		Rating:      si.ItemRating.RatingStar,
		Sold:        si.HistoricalSold,
	}
}
