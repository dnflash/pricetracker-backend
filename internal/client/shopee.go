package client

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"strings"
)

type ShopeeItemResponseData struct {
	ShopID string
	ItemID string
	Name   string
	Price  int
	Stock  int
}

func (c Client) ShopeeGetItem(url string) (ShopeeItemResponseData, error) {
	shopID, itemID := shopeeGetShopAndItemID(url)
	apiURL := fmt.Sprintf("https://shopee.co.id/api/v4/item/get?shopid=%s&itemid=%s", shopID, itemID)

	req, err := newGetRequest(apiURL, nil)
	if err != nil {
		return ShopeeItemResponseData{}, errors.Wrapf(err, "error creating request from apiURL: %+v", apiURL)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return ShopeeItemResponseData{}, errors.Wrapf(err, "error doing request: %+v", req)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Error("error closing response body", errors.Wrap(err, "error closing response body"))
		}
	}()

	body := map[string]any{}
	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ShopeeItemResponseData{}, errors.Wrapf(err, "error decoding ShopeeItemAPI response body, apiURL: %+v", apiURL)
	}
	body = body["data"].(map[string]any)

	return ShopeeItemResponseData{
		ShopID: shopID,
		ItemID: itemID,
		Name:   body["name"].(string),
		Price:  int(body["price"].(float64)),
		Stock:  int(body["price"].(float64)),
	}, nil
}

func shopeeGetShopAndItemID(url string) (shopID string, itemID string) {
	sp := strings.Split(url, ".")
	return sp[len(sp)-2], sp[len(sp)-1]
}
