package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v9"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"net/url"
	"pricetracker/internal/misc"
	"pricetracker/internal/model"
	"strconv"
	"strings"
	"time"
)

var ErrBlibli = errors.New("Blibli error")
var ErrBlibliItemNotFound = errors.New("Blibli item not found")

type blibliProductDetailResponse struct {
	Code int                     `json:"code"`
	Data blibliProductDetailData `json:"data"`
}

type blibliProductDetailData struct {
	URL             string `json:"url"`
	ItemSKU         string `json:"itemSku"`
	Name            string `json:"name"`
	ProductSKU      string `json:"productSku"`
	URLFriendlyName string `json:"urlFriendlyName"`
	Stock           int    `json:"stock"`
	Price           struct {
		Listed  float64 `json:"listed"`
		Offered float64 `json:"offered"`
	} `json:"price"`
	Images []struct {
		Full      string `json:"full"`
		Thumbnail string `json:"thumbnail"`
	} `json:"images"`
	Merchant struct {
		Name string `json:"name"`
		Code string `json:"code"`
	} `json:"merchant"`
	Review struct {
		DecimalRating float64 `json:"decimalRating"`
	} `json:"review"`
	Statistics struct {
		Sold int `json:"sold"`
	} `json:"statistics"`
}

type blibliProductDescriptionResponse struct {
	Code int `json:"code"`
	Data struct {
		Value string `json:"value"`
	} `json:"data"`
}

func (c Client) BlibliGetItem(url string, useCache bool) (model.Item, error) {
	ctx := context.TODO()
	var i model.Item
	sku, err := c.blibliGetSKU(url)
	if err != nil {
		return i, fmt.Errorf("%w: failed getting SKU from URL: %#v, err: %v", ErrBlibliItemNotFound, url, err)
	}
	apiURL := fmt.Sprintf("https://www.blibli.com/backend/product-detail/products/%s/_summary", sku)
	cacheKey := "BGI-" + apiURL
	if useCache {
		cached, err := c.Redis.Get(ctx, cacheKey).Result()
		if err == nil {
			c.Logger.Infof("BlibliGetItem: Cache found, key: %s", cacheKey)
			if err = json.Unmarshal([]byte(cached), &i); err == nil {
				return i, nil
			} else {
				c.Logger.Errorf("BlibliGetItem: Error unmarshalling cache, key: %s, err: %v", cacheKey, err)
			}
		} else {
			if err != redis.Nil {
				c.Logger.Errorf("BlibliGetItem: Error getting getting Redis cache with key: %s, err: %v", cacheKey, err)
			}
		}
	}

	req, err := newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return i, fmt.Errorf("failed to create request to URL: %s, err: %v", apiURL, err)
	}
	req.Header.Add("Accept-Language", "en")

	c.Logger.Infof("BlibliGetItem: Sending request to %s", apiURL)
	resp, err := c.Do(req)
	if err != nil {
		return i, fmt.Errorf("%w: error doing request:\n%#v,\nerr: %v", ErrBlibli, req, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 300*1024))
	if err != nil {
		return i, fmt.Errorf(
			"error reading BlibliProductAPI response body, status: %s, body:\n%s,\nerr: %v",
			resp.Status, misc.BytesLimit(body, 2000), err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return i, fmt.Errorf("%w: status: %s, body:\n%s",
			ErrBlibliItemNotFound, resp.Status, misc.BytesLimit(body, 2000))
	}
	blibliResp := blibliProductDetailResponse{}
	if err = json.Unmarshal(body, &blibliResp); err != nil {
		return i, fmt.Errorf(
			"error unmarshalling BlibliProductAPI response body, status: %s, body:\n%s,\nerr: %v",
			resp.Status, misc.BytesLimit(body, 2000), err)
	}
	if blibliResp.Code != 200 {
		return i, fmt.Errorf("error getting data from BlibliProductAPI, status: %s, body:\n%s",
			resp.Status, misc.BytesLimit(body, 2000))
	}
	i = blibliResp.Data.toItem()
	if i.ProductID == "" || i.URL == "" || i.ImageURL == "" {
		return i, fmt.Errorf("error parsing Blibli product: %+v, Item: %+v", blibliResp.Data, i)
	}
	i.Description, err = c.blibliGetItemDescription(i.ProductID)
	if err != nil {
		return i, fmt.Errorf("error getting Blibli product description, Item: %+v, err: %w", i, err)
	}

	if iJSON, err := json.Marshal(i); err != nil {
		c.Logger.Errorf("BlibliGetItem: Error marshalling Item to cache, key: %s, Item: %+v, err: %v", cacheKey, i, err)
	} else {
		if err = c.Redis.Set(ctx, cacheKey, iJSON, 1*time.Hour).Err(); err != nil {
			c.Logger.Errorf("BlibliGetItem: Error caching Item, key: %s, Item: %+v, err: %v", cacheKey, i, err)
		}
	}

	return i, nil
}

func (c Client) blibliGetItemDescription(sku string) (string, error) {
	normSKU, ok := blibliNormalizeSKU(sku)
	if !ok || len(normSKU) != 21 {
		return "", fmt.Errorf("invalid SKU: %#v", sku)
	}
	apiURL := fmt.Sprintf("https://www.blibli.com/backend/product-detail/products/%s/description", sku)
	req, err := newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request to URL: %s, err: %v", apiURL, err)
	}
	req.Header.Add("Accept-Language", "en")
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: error doing request:\n%#v,\nerr: %v", ErrBlibli, req, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 100*1024))
	if err != nil {
		return "", fmt.Errorf(
			"error reading BlibliProductDescriptionAPI response body, status: %s, body:\n%s,\nreq:\n%#v,\nerr: %v",
			resp.Status, misc.BytesLimit(body, 2000), req, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%w: status: %s, body:\n%s,\nreq:\n%#v",
			ErrBlibliItemNotFound, resp.Status, misc.BytesLimit(body, 2000), req)
	}
	blibliResp := blibliProductDescriptionResponse{}
	if err = json.Unmarshal(body, &blibliResp); err != nil {
		return "", fmt.Errorf(
			"error unmarshalling BlibliProductDescriptionAPI response body, status: %s, body:\n%s,\nreq:\n%#v,\nerr: %v",
			resp.Status, misc.BytesLimit(body, 2000), req, err)
	}
	if blibliResp.Code != 200 {
		return "", fmt.Errorf("error getting data from BlibliProductDescriptionAPI, status: %s, body:\n%s,\nreq:\n%#v",
			resp.Status, misc.BytesLimit(body, 2000), req)
	}
	return blibliDescriptionParser(blibliResp.Data.Value)
}

func blibliDescriptionParser(s string) (string, error) {
	node, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return "", fmt.Errorf("failed to parse description HTML, err: %v", err)
	}
	bodyNode, err := htmlBodyFinder(node)
	if err != nil {
		return "", fmt.Errorf("failed to find description HTML body, err: %v", err)
	}
	bodyBuf := &bytes.Buffer{}
	bodyBuf.Grow(len(s))
	if err = html.Render(bodyBuf, bodyNode); err != nil {
		return "", fmt.Errorf("failed to render description HTML body, err: %v", err)
	}
	body := bodyBuf.Bytes()
	body = bytes.ReplaceAll(body, []byte("\\n"), []byte(""))
	body = bytes.ReplaceAll(body, []byte("<br/>"), []byte("\n"))
	body = misc.HTMLTagRegex.ReplaceAllLiteral(body, []byte(" "))
	body = misc.ExtraSpaceRegex.ReplaceAllLiteral(body, []byte(" "))
	body = bytes.TrimSpace(body)
	return html.UnescapeString(string(body)), nil
}

func htmlBodyFinder(node *html.Node) (*html.Node, error) {
	if node == nil {
		return nil, errors.New("input node is nil")
	}
	for c, i := node, 0; c != nil && i < 300; i++ {
		if c.Type == html.ElementNode && c.Data == "body" {
			return c, nil
		} else if c.FirstChild != nil {
			c = c.FirstChild
		} else if c.NextSibling != nil {
			c = c.NextSibling
		} else if c.Parent != nil && c != node {
			var j int
			for c = c.Parent; c != nil; c, j = c.Parent, j+1 {
				if j == 20 {
					return nil, errors.New("ascend limit exceeded")
				}
				if c == node {
					return nil, errors.New("body not found")
				}
				if c.NextSibling != nil {
					c = c.NextSibling
					break
				}
			}
		} else {
			return nil, errors.New("node has no path to traverse")
		}
	}
	return nil, errors.New("traverse limit exceeded")
}

func (c Client) blibliGetSKU(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("error parsing URL: %v", err)
	}
	if parsedURL.Host == "blibli.app.link" && len(parsedURL.Path) > 5 {
		if resolvedURL, err := c.blibliResolveShareLink("https://blibli.app.link" + parsedURL.Path); err != nil {
			return "", fmt.Errorf("failed to get SKU from share link, err: %v", err)
		} else if parsedURL, err = url.Parse(resolvedURL); err != nil {
			return "", fmt.Errorf("error parsing resolved URL from share link, err: %v", err)
		}
	}
	if parsedURL.Host == "www.blibli.com" || parsedURL.Host == "blibli.com" {
		sp := strings.Split(parsedURL.Path, "/")
		if len(sp) == 4 || len(sp) == 5 {
			sku := sp[len(sp)-1]
			if normSKU, ok := blibliNormalizeSKU(sku); ok {
				return normSKU, nil
			}
			return "", fmt.Errorf("invalid SKU: %#v, from URL: %s", sku, parsedURL)
		}
	}
	return "", fmt.Errorf("invalid URL: %s", parsedURL)
}

func (c Client) blibliResolveShareLink(url string) (string, error) {
	ctx := context.TODO()
	cacheKey := "BRSL-" + url
	cached, err := c.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		c.Logger.Infof("blibliResolveShareLink: Cache found, key: %s", cacheKey)
		return cached, nil
	} else {
		if err != redis.Nil {
			c.Logger.Errorf("blibliResolveShareLink: Error getting getting Redis cache with key: %s, err: %v", cacheKey, err)
		}
	}

	req, err := newRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request from URL: %s, err: %v", url, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 Windows")
	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error doing request, req:\n%#v,\nerr: %v", req, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	bodyRdr := io.LimitReader(resp.Body, 500*1024)
	if resp.StatusCode != http.StatusTemporaryRedirect {
		body, _ := io.ReadAll(bodyRdr)
		return "", fmt.Errorf(
			"failed resolving share link, url: %s, status is not %d, resp:\n%#v,\nbody:\n%s,\nreq:\n%#v",
			url, http.StatusTemporaryRedirect, resp, misc.BytesLimit(body, 1000), req)
	}
	_, _ = io.Copy(io.Discard, bodyRdr)

	location := resp.Header.Get("Location")
	if err = c.Redis.Set(ctx, cacheKey, location, 72*time.Hour).Err(); err != nil {
		c.Logger.Errorf("blibliResolveShareLink: Error caching resolved URL, key: %s, URL: %s, err: %v", cacheKey, location, err)
	}

	return location, nil
}

func blibliNormalizeSKU(sku string) (string, bool) {
	if len(sku) >= 15 {
		prefix := strings.ToLower(sku[:4])
		if prefix == "ps--" || prefix == "is--" {
			sku = sku[4:]
		} else {
			prefix = ""
		}
		if ((prefix == "" || prefix == "ps--") && len(sku) == 15) ||
			((prefix == "" || prefix == "is--") && len(sku) == 21) {
			sku = strings.ToUpper(strings.ReplaceAll(sku, ".", "-"))
			if sku[3] != '-' || !misc.IsNum(sku[4:9]) || sku[9] != '-' || !misc.IsNum(sku[10:15]) {
				return "", false
			}
			if len(sku) == 21 && (sku[15] != '-' || !misc.IsNum(sku[16:21])) {
				return "", false
			}
			return prefix + sku, true
		}
	}
	return "", false
}

func (bp blibliProductDetailData) toItem() model.Item {
	var itemURL string
	normItemSKU, _ := blibliNormalizeSKU(bp.ItemSKU)
	if len(normItemSKU) != 21 {
		normItemSKU = ""
	}
	if normItemSKU != "" {
		itemURL = fmt.Sprintf("https://www.blibli.com/p/%s/is--%s",
			strings.ToLower(strings.ReplaceAll(misc.CleanString(bp.Name), " ", "-")), normItemSKU)
	}
	itemName := strings.TrimSpace(strings.ReplaceAll(bp.Name, "\n", " "))
	var imageURL string
	if len(bp.Images) > 0 {
		s := bp.Images[0].Full
		if strings.HasPrefix(s, "https://www.static-src.com/") {
			imageURL = s
		}
	}
	return model.Item{
		Site:        "Blibli",
		MerchantID:  normItemSKU[:misc.Min(9, len(normItemSKU))],
		ProductID:   normItemSKU,
		ParentID:    normItemSKU[:misc.Min(15, len(normItemSKU))],
		VariationID: normItemSKU,
		URL:         itemURL,
		Name:        itemName,
		Price:       int(bp.Price.Offered),
		Stock:       bp.Stock,
		ImageURL:    imageURL,
		Description: "",
		Rating:      bp.Review.DecimalRating,
		Sold:        bp.Statistics.Sold,
	}
}

type blibliSearchResponse struct {
	Code int `json:"code"`
	Data struct {
		Products []blibliSearchProduct `json:"products"`
	} `json:"data"`
}

type blibliSearchProduct struct {
	MerchantCode string `json:"merchantCode"`
	ItemSKU      string `json:"itemSku"`
	Name         string `json:"name"`
	Price        struct {
		MinPrice float64 `json:"minPrice"`
	} `json:"price"`
	Images []string `json:"images"`
	Review struct {
		AbsoluteRating float64 `json:"absoluteRating"`
	} `json:"review"`
	SoldRangeCount struct {
		ID string `json:"id"`
	} `json:"soldRangeCount"`
}

func (c Client) BlibliSearch(query string) ([]model.Item, error) {
	ctx := context.TODO()
	var is []model.Item
	apiURL := "https://www.blibli.com/backend/search/products"

	cacheKey := "BS-" + query
	cached, err := c.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		c.Logger.Infof("BlibliSearch: Cache found, key: %s", cacheKey)
		if err = json.Unmarshal([]byte(cached), &is); err == nil {
			return is, nil
		} else {
			c.Logger.Errorf("BlibliSearch: Error unmarshalling cache, key: %s, err: %v", cacheKey, err)
		}
	} else {
		if err != redis.Nil {
			c.Logger.Errorf("BlibliSearch: Error getting getting Redis cache with key: %s, err: %v", cacheKey, err)
		}
	}

	req, err := newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return is, fmt.Errorf("failed to create request to URL: %s, err: %v", apiURL, err)
	}
	qp := url.Values{
		"intent":         []string{"true"},
		"searchTerm":     []string{query},
		"channelId":      []string{"web"},
		"showFacet":      []string{"false"},
		"userIdentifier": []string{"undefined"},
	}.Encode()
	req.URL.RawQuery = strings.ReplaceAll(qp, "+", "%20")
	req.Header.Add("Accept-Language", "en")

	c.Logger.Infof("BlibliSearch: Sending request to %s", apiURL)
	resp, err := c.Client.Do(req)
	if err != nil {
		return is, fmt.Errorf("%w: error doing request:\n%#v,\nerr: %v", ErrBlibli, req, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	blibliSearchResp := blibliSearchResponse{}
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 1000*1024))
	if err != nil {
		return is, fmt.Errorf(
			"error reading BlibliSearchAPI response body, status: %s, body:\n%s,\nerr: %v",
			resp.Status, misc.BytesLimit(body, 2000), err)
	}
	if err = json.Unmarshal(body, &blibliSearchResp); err != nil {
		return is, fmt.Errorf(
			"error unmarshalling BlibliSearchAPI response body, status: %s, body:\n%s,\nerr: %v",
			resp.Status, misc.BytesLimit(body, 2000), err)
	}
	if blibliSearchResp.Code == 422 {
		return is, fmt.Errorf("%w: error getting data from BlibliSearchAPI, status: %s, body:\n%s",
			ErrBlibliItemNotFound, resp.Status, misc.BytesLimit(body, 200))
	}
	if blibliSearchResp.Code != 200 {
		return is, fmt.Errorf("%w: error getting data from BlibliSearchAPI, status: %s, body:\n%s",
			ErrBlibli, resp.Status, misc.BytesLimit(body, 2000))
	}
	bsps := blibliSearchResp.Data.Products
	is = make([]model.Item, 0, len(bsps))
	for _, bsp := range bsps[:misc.Min(10, len(bsps))] {
		i := bsp.toItem()
		if i.ProductID == "" || i.URL == "" || i.ImageURL == "" {
			c.Logger.Warnf("BlibliSearch: Error parsing Blibli product: %+v, Item: %+v", bsp, i)
			continue
		}
		is = append(is, i)
	}

	if isJSON, err := json.Marshal(is); err != nil {
		c.Logger.Errorf("BlibliSearch: Error marshalling Items to cache, key: %s, Item: %+v, err: %v", cacheKey, is, err)
	} else {
		if err = c.Redis.Set(ctx, cacheKey, isJSON, 12*time.Hour).Err(); err != nil {
			c.Logger.Errorf("BlibliSearch: Error caching Items, key: %s, Item: %+v, err: %v", cacheKey, is, err)
		}
	}

	return is, nil
}

func (bsp blibliSearchProduct) toItem() model.Item {
	var itemURL string
	normItemSKU, _ := blibliNormalizeSKU(bsp.ItemSKU)
	if len(normItemSKU) != 21 {
		normItemSKU = ""
	}
	if normItemSKU != "" {
		itemURL = fmt.Sprintf("https://www.blibli.com/p/%s/is--%s",
			strings.ToLower(strings.ReplaceAll(misc.CleanString(bsp.Name), " ", "-")), normItemSKU)
	}
	itemName := strings.TrimSpace(strings.ReplaceAll(bsp.Name, "\n", " "))
	var imageURL string
	if len(bsp.Images) > 0 {
		s := bsp.Images[0]
		if strings.HasPrefix(s, "https://www.static-src.com/") {
			imageURL = s
		}
	}
	return model.Item{
		Site:        "Blibli",
		MerchantID:  normItemSKU[:misc.Min(9, len(normItemSKU))],
		ProductID:   normItemSKU,
		ParentID:    normItemSKU[:misc.Min(15, len(normItemSKU))],
		VariationID: normItemSKU,
		URL:         itemURL,
		Name:        itemName,
		Price:       int(bsp.Price.MinPrice),
		Stock:       -1,
		ImageURL:    imageURL,
		Description: "",
		Rating:      bsp.Review.AbsoluteRating,
		Sold:        blibliSoldParser(bsp.SoldRangeCount.ID),
	}
}

func blibliSoldParser(s string) int {
	if strings.HasSuffix(s, " rb") || strings.HasSuffix(s, " jt") {
		if a, b, ok := strings.Cut(s[:len(s)-3], ","); ok {
			if sold, err := strconv.Atoi(a); err == nil {
				if trail, err := strconv.ParseFloat("0."+b, 64); err == nil {
					if strings.HasSuffix(s, " rb") {
						return sold*1000 + int(trail*1000)
					}
					return sold*1000000 + int(trail*1000000)
				}
			}
		} else if sold, err := strconv.Atoi(s[:len(s)-3]); err == nil {
			if strings.HasSuffix(s, " rb") {
				return sold * 1000
			}
			return sold * 1000000
		}
	} else if sold, err := strconv.Atoi(s); err == nil {
		return sold
	}
	return 0
}
