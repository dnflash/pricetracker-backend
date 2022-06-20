package client

import (
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"pricetracker/internal/misc"
	"pricetracker/internal/model"
	"strconv"
	"strings"
)

var ErrTokopedia = errors.New("Tokopedia error")
var ErrTokopediaItemNotFound = errors.New("Tokopedia item not found")

var errTokopediaNotPDP = errors.New("Tokopedia page is not PDP")
var errTokopediaFieldKeyNotFound = errors.New("Tokopedia field key not found")

func (c Client) TokopediaGetItem(url string) (model.Item, error) {
	var i model.Item
	normURL, isShareLink, err := tokopediaNormalizeURL(url)
	if err != nil {
		return i, fmt.Errorf("%w: error normalizing URL, err: %v", ErrTokopediaItemNotFound, err)
	}
	if isShareLink {
		normURL, err = c.tokopediaResolveShareLink(normURL)
		if err != nil {
			return i, fmt.Errorf("%w: error resolving share link, err: %v", ErrTokopediaItemNotFound, err)
		}
	}
	req, err := newRequest(http.MethodGet, normURL, nil)
	if err != nil {
		return i, errors.Wrapf(err, "error creating request from URL: %s", normURL)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return i, errors.Wrapf(ErrTokopedia, "error doing request:\n%#v,\nerr: %v", req, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.Logger.Errorf("TokopediaGetItem: Error closing response body, resp:\n%#v,\nreq:\n%#v,\nerr: %v", resp, req, err)
		}
	}()

	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 1024*1024))
	if err != nil {
		return i, errors.Wrapf(err,
			"error reading Tokopedia product page response body, status: %s, body:\n%s,\nreq:\n%#v",
			resp.Status, misc.BytesLimit(body, 500), req)
	}

	if resp.StatusCode == http.StatusGone {
		return i, errors.Wrapf(ErrTokopediaItemNotFound,
			"Tokopedia item not found, status: %s, body:\n%s,\nreq:\n%#v",
			resp.Status, misc.BytesLimit(body, 500), req)
	}

	if resp.StatusCode != http.StatusOK {
		return i, errors.Wrapf(ErrTokopedia, "error getting item from Tokopedia, status: %s, body:\n%s,\nreq:\n%#v",
			resp.Status, misc.BytesLimit(body, 500), req)
	}

	i, err = tokopediaParseProductPage(body)
	if err != nil {
		if errors.Is(err, errTokopediaNotPDP) {
			return i, errors.Wrapf(ErrTokopediaItemNotFound, "%v", err)
		}
		return i, errors.Wrapf(err, "error parsing product page, status: %s, body:\n%s,\nreq:\n%#v",
			resp.Status, misc.BytesLimit(body, 500), req)
	}
	return i, nil
}

func tokopediaNormalizeURL(urlStr string) (string, bool, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", false, err
	}
	if parsedURL.Host == "www.tokopedia.com" || parsedURL.Host == "tokopedia.com" {
		sp := strings.Split(parsedURL.Path, "/")
		if len(sp) >= 3 {
			return "https://www.tokopedia.com" + strings.Join(sp[:3], "/"), false, nil
		} else {
			return "", false, errors.Errorf("invalid url: %s", urlStr)
		}
	} else if parsedURL.Host == "tokopedia.link" && len(parsedURL.Path) > 5 {
		return "https://tokopedia.app.link" + parsedURL.Path, true, nil
	} else {
		return "", false, errors.Errorf("invalid url: %s", urlStr)
	}
}

func (c Client) tokopediaResolveShareLink(url string) (string, error) {
	req, err := newRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request from URL: %s, err: %w", url, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 Windows")
	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error doing request, req:\n%#v,\nerr: %w", req, err)
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
	normURL, isShareLink, err := tokopediaNormalizeURL(resp.Header.Get("Location"))
	if err != nil {
		return "", fmt.Errorf("failed resolving share link, err: %w", err)
	} else if isShareLink {
		return "", fmt.Errorf("failed resolving share link: recursive")
	}
	return normURL, nil
}

func tokopediaParseProductPage(pageBytes []byte) (model.Item, error) {
	var i model.Item
	pdpSessionIdx := bytes.Index(pageBytes, []byte("pdpSession\":\"{\\\""))
	if pdpSessionIdx < 0 {
		return i, errors.Wrapf(errTokopediaNotPDP, "PDP session not found")
	}
	page := string(pageBytes[pdpSessionIdx+len("pdpSession\":\"{\\\""):])

	merchantID, err := tokopediaFindValue(page, "sid\\\":", ",", false, 32)
	if err != nil {
		return i, errors.Wrapf(err, "failed to find merchantID")
	} else if _, err = strconv.Atoi(merchantID); err != nil {
		return i, errors.Wrapf(err, "invalid merchantID")
	}

	productID, err := tokopediaFindValue(page, "pi\\\":", ",", false, 32)
	if err != nil {
		return i, errors.Wrapf(err, "failed to find productID")
	} else if _, err = strconv.Atoi(productID); err != nil {
		return i, errors.Wrapf(err, "invalid productID")
	}

	parentID, err := tokopediaFindValue(page, "pi\\\":", ",", false, 32)
	if err != nil {
		parentID = ""
	} else if _, err = strconv.Atoi(parentID); err != nil {
		return i, errors.Wrapf(err, "invalid parentID")
	}

	shopHandle, err := tokopediaFindValue(page, "sd\\\":", ",", true, 32)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting shopHandle")
	}
	urlPart, err := tokopediaFindValue(page, "alias\":", ",", true, 300)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting urlPart")
	}

	itemName, err := tokopediaFindValue(page, "pn\\\":", ",\\\"", true, 300)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting itemName")
	}
	_, variationID, _ := strings.Cut(itemName, " - ")

	var itemPrice int
	itemPriceStr, err := tokopediaFindValue(page, "pr\\\":", ",", false, 32)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting itemPrice")
	} else if itemPrice, err = strconv.Atoi(itemPriceStr); err != nil {
		return i, errors.Wrapf(err, "invalid itemPrice")
	}

	var itemStock int
	itemStockStr, err := tokopediaFindValue(page, "st\\\":", ",", false, 32)
	if err != nil {
		if errors.Is(err, errTokopediaFieldKeyNotFound) {
			itemStockStr = "0"
		} else {
			return i, errors.Wrapf(err, "failed getting itemStock")
		}
	} else if itemStock, err = strconv.Atoi(itemStockStr); err != nil {
		return i, errors.Wrapf(err, "invalid itemStock")
	}

	imageURL, err := tokopediaFindValue(page, "{\"type\":\"image\",\"URLThumbnail\":", ",", true, 200)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting imageURL")
	}
	imageURL = strings.Replace(imageURL, "/200-square/", "/500-square/", 1)

	itemDescription, err := tokopediaFindValue(page, "{\"title\":\"Deskripsi\",\"subtitle\":", ",\"", true, 3500)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting itemDescription")
	}

	var itemRating float64
	itemRatingStr, err := tokopediaFindValue(page, "rating\":", ",", false, 64)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting itemRating")
	} else if itemRating, err = strconv.ParseFloat(itemRatingStr, 64); err != nil {
		return i, errors.Wrapf(err, "invalid itemRating")
	}

	var itemSold int
	itemSoldStr, err := tokopediaFindValue(page, "countSold\":", ",", true, 32)
	if err != nil {
		return i, errors.Wrapf(err, "failed getting itemSold")
	} else if itemSold, err = strconv.Atoi(itemSoldStr); err != nil {
		return i, errors.Wrapf(err, "invalid itemSold")
	}

	return model.Item{
		Site:        "Tokopedia",
		MerchantID:  merchantID,
		ProductID:   productID,
		ParentID:    parentID,
		VariationID: variationID,
		URL:         fmt.Sprintf("www.tokopedia.com/%s/%s", shopHandle, urlPart),
		Name:        itemName,
		Price:       itemPrice,
		Stock:       itemStock,
		ImageURL:    imageURL,
		Description: misc.StringLimit(itemDescription, 2500),
		Rating:      itemRating,
		Sold:        itemSold,
	}, nil
}

func tokopediaFindValue(page string, key string, sep string, unquote bool, maxLength int) (string, error) {
	keyIdx := strings.Index(page, key)
	if keyIdx < 0 {
		return "", errors.Wrapf(errTokopediaFieldKeyNotFound, "key (%#v) not found, page: %#v",
			key, misc.StringLimit(page, misc.Max(maxLength+100, 250)))
	}
	page = page[keyIdx+len(key):]
	page = page[:maxLength+1000]
	sepIdx := strings.Index(page, sep)
	clBrIdx := strings.Index(page, "}")
	if clBrIdx >= 0 && clBrIdx < sepIdx {
		opBrIdx := strings.Index(page, "{")
		if opBrIdx < 0 || opBrIdx > clBrIdx {
			sep = "}"
		}
	} else if sepIdx < 0 && clBrIdx >= 0 {
		opBrIdx := strings.Index(page, "{")
		if opBrIdx >= 0 && opBrIdx < clBrIdx {
			return "", errors.Errorf("failed to find value for key (%#v), sep (%#v) not found and \"}\" substitute is invalid, page: %#v",
				key, sep, misc.StringLimit(page, misc.Max(maxLength+100, 250)))
		}
		sep = "}"
	}
	val, _, ok := strings.Cut(page, sep)
	if ok {
		val = strings.ReplaceAll(val, "\\\"", "\"")
		if unquote {
			unqVal, err := strconv.Unquote(val)
			if err != nil {
				return "", errors.Wrapf(err, "failed unquoting value for key (%#v), val: %#v",
					key, misc.StringLimit(val, misc.Max(maxLength+100, 250)))
			} else {
				val = unqVal
			}
		}
		val = html.UnescapeString(val)
		if len(val) > maxLength {
			return "", errors.Errorf("value for key (%#v) too long (max: %d), val: %#v",
				key, maxLength, misc.StringLimit(val, misc.Max(maxLength+100, 250)))
		}
		return val, nil
	}
	return "", errors.Errorf("failed to find value for key (%#v), sep (%#v) not found, page: %#v",
		key, sep, misc.StringLimit(page, misc.Max(maxLength+100, 250)))
}
