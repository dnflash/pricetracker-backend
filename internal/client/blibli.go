package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"net/url"
	"pricetracker/internal/misc"
	"pricetracker/internal/model"
	"strings"
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

func (c Client) BlibliGetItem(url string) (model.Item, error) {
	var i model.Item
	sku, err := c.blibliGetSKU(url)
	if err != nil {
		return i, fmt.Errorf("%w: failed getting SKU from URL: %#v, err: %v", ErrBlibliItemNotFound, url, err)
	}
	apiURL := fmt.Sprintf("https://www.blibli.com/backend/product-detail/products/%s/_summary", sku)
	req, err := newRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return i, fmt.Errorf("failed to create request to URL: %s, err: %v", apiURL, err)
	}
	req.Header.Add("Accept-Language", "en")
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
			"error reading BlibliProductAPI response body, status: %s, body:\n%s,\nreq:\n%#v,\nerr: %v",
			resp.Status, body, req, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return i, fmt.Errorf("%w: status: %s, body:\n%s,\nreq:\n%#v",
			ErrBlibliItemNotFound, resp.Status, body, req)
	}
	blibliResp := blibliProductDetailResponse{}
	if err = json.Unmarshal(body, &blibliResp); err != nil {
		return i, fmt.Errorf(
			"error unmarshalling BlibliProductAPI response body, status: %s, body:\n%s,\nreq:\n%#v,\nerr: %v",
			resp.Status, body, req, err)
	}
	if blibliResp.Code != 200 {
		return i, fmt.Errorf("error getting data from BlibliProductAPI, status: %s, body:\n%s,\nreq:\n%#v",
			resp.Status, body, req)
	}
	i = blibliResp.Data.toItem()
	if i.ProductID == "" || i.URL == "" || i.ImageURL == "" {
		return i, fmt.Errorf("error parsing Blibli product: %+v, Item: %+v", blibliResp.Data, i)
	}
	i.Description, err = c.blibliGetItemDescription(i.ProductID)
	if err != nil {
		return i, fmt.Errorf("error getting Blibli product description, Item: %+v, err: %w", i, err)
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
			resp.Status, body, req, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%w: status: %s, body:\n%s,\nreq:\n%#v",
			ErrBlibliItemNotFound, resp.Status, body, req)
	}
	blibliResp := blibliProductDescriptionResponse{}
	if err = json.Unmarshal(body, &blibliResp); err != nil {
		return "", fmt.Errorf(
			"error unmarshalling BlibliProductDescriptionAPI response body, status: %s, body:\n%s,\nreq:\n%#v,\nerr: %v",
			resp.Status, body, req, err)
	}
	if blibliResp.Code != 200 {
		return "", fmt.Errorf("error getting data from BlibliProductDescriptionAPI, status: %s, body:\n%s,\nreq:\n%#v",
			resp.Status, body, req)
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
		return "", fmt.Errorf("failed resolving share link, url: %s, status is not %d, resp:\n%#v,\nbody:\n%s,\nreq:\n%#v",
			url, http.StatusTemporaryRedirect, resp, misc.BytesLimit(body, 500), req)
	}
	_, _ = io.Copy(io.Discard, bodyRdr)
	return resp.Header.Get("Location"), nil
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
		MerchantID:  bp.Merchant.Code,
		ProductID:   normItemSKU,
		ParentID:    bp.ProductSKU,
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
