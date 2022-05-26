package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var ErrShopeeItemNotFound = errors.New("Shopee item not found")

type ShopeeItemResponse struct {
	Error int                     `json:"error"`
	Data  *ShopeeItemResponseData `json:"data"`
}

type ShopeeItemResponseData struct {
	ShopID   int    `json:"shopid"`
	ItemID   int    `json:"itemid"`
	Name     string `json:"name"`
	Price    int    `json:"price"`
	Stock    int    `json:"stock"`
	ImageURL string `json:"image"`
}

func (c Client) ShopeeGetItem(url string) (ShopeeItemResponseData, error) {
	shopID, itemID, ok := shopeeGetShopAndItemID(url)
	if !ok {
		return ShopeeItemResponseData{}, errors.Errorf("error getting ShopID and ItemID from URL: %s", url)
	}
	apiURL := fmt.Sprintf("https://shopee.co.id/api/v4/item/get?shopid=%s&itemid=%s", shopID, itemID)

	req, err := newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return ShopeeItemResponseData{}, errors.Wrapf(err, "error creating request from apiURL: %s", apiURL)
	}
	req.AddCookie(&http.Cookie{
		Name:  "SPC_U",
		Value: "-",
	})
	resp, err := c.Client.Do(req)
	if err != nil {
		return ShopeeItemResponseData{}, errors.Wrapf(err, "error doing request: %+v", req)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Error("error closing response body", errors.Wrapf(err, "error closing response body on request to url: %s", req.URL))
		}
	}()

	shopeeItemResp := ShopeeItemResponse{}
	bodyReader := http.MaxBytesReader(nil, resp.Body, 300000)
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return ShopeeItemResponseData{}, errors.Wrapf(err, "error reading ShopeeItemAPI response body, apiURL: %s", apiURL)
	}
	if err = json.NewDecoder(bytes.NewReader(body)).Decode(&shopeeItemResp); err != nil {
		return ShopeeItemResponseData{}, errors.Wrapf(err,
			"error decoding ShopeeItemAPI response body, apiURL: %s, body:\n%+v", apiURL, string(body))
	}

	if shopeeItemResp.Error != 0 || shopeeItemResp.Data == nil {
		return ShopeeItemResponseData{}, ErrShopeeItemNotFound
	}

	shopeeItemResp.Data.Price /= 100000
	shopeeItemResp.Data.ImageURL = "https://cf.shopee.co.id/file/" + shopeeItemResp.Data.ImageURL

	return *shopeeItemResp.Data, nil
}

func shopeeGetShopAndItemID(urlStr string) (shopID string, itemID string, ok bool) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", false
	}
	if strings.HasPrefix(parsedURL.Path, "/product/") {
		sp := strings.Split(parsedURL.Path, "/")
		return sp[len(sp)-2], sp[len(sp)-1], true
	}
	sp := strings.Split(parsedURL.Path, ".")
	return sp[len(sp)-2], sp[len(sp)-1], true
}
