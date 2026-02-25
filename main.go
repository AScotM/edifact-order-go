package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SegmentTagUNB = "UNB"
	SegmentTagUNH = "UNH"
	SegmentTagBGM = "BGM"
	SegmentTagDTM = "DTM"
	SegmentTagCUX = "CUX"
	SegmentTagNAD = "NAD"
	SegmentTagTOD = "TOD"
	SegmentTagPAT = "PAT"
	SegmentTagTDT = "TDT"
	SegmentTagLIN = "LIN"
	SegmentTagIMD = "IMD"
	SegmentTagQTY = "QTY"
	SegmentTagPRI = "PRI"
	SegmentTagMOA = "MOA"
	SegmentTagUNS = "UNS"
	SegmentTagCNT = "CNT"
	SegmentTagUNT = "UNT"
	SegmentTagUNZ = "UNZ"
	
	DateFormatYYMMDD = "060102"
	DateFormatHHMM   = "1504"
	DateFormatCCYYMMDD = "20060102"
	
	QualifierDocumentDate = "137"
	QualifierDeliveryDate = "2"
	QualifierLineDeliveryDate = "64"
	
	CodeOrder = "220"
	CodeOriginal = "9"
	
	PartyBuyer = "BY"
	PartySeller = "SE"
	PartyDelivery = "DP"
	PartyInvoice = "IV"
	
	IDTypeBuyer = "9"
	
	CurrencyReference = "2"
	
	QuantityOrdered = "21"
	
	PriceNet = "AAA"
	
	AmountLine = "203"
	AmountTotal = "128"
	
	ControlTotalLines = "2"
	
	FilePerms = 0644
	DirPerms = 0755
)

var (
	ErrInvalidOrder = errors.New("invalid order data")
	ErrMissingField = errors.New("required field missing")
	ErrFileWrite = errors.New("failed to write file")
)

type ValidationError struct {
	Field string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %s: %s", e.Field, e.Message)
}

type EDISegment struct {
	Tag      string
	Elements []string
}

func (s EDISegment) String(separator string, terminator string) string {
	return s.Tag + separator + strings.Join(s.Elements, separator) + terminator
}

type Address struct {
	Name    string
	Lines   []string
	ID      string
	IDType  string
}

func (a Address) Validate() error {
	if a.Name == "" {
		return &ValidationError{Field: "Address.Name", Message: "name is required"}
	}
	if len(a.Lines) == 0 {
		return &ValidationError{Field: "Address.Lines", Message: "at least one address line is required"}
	}
	return nil
}

type EDIOrderItem struct {
	LineNumber      int
	BuyerItemCode   string
	SupplierItemCode string
	Quantity        float64
	UnitPrice       float64
	UnitOfMeasure   string
	Description     string
	TaxRate         float64
	Amount          float64
	DeliveryDate    time.Time
}

func (i EDIOrderItem) Validate() error {
	if i.LineNumber <= 0 {
		return &ValidationError{Field: "EDIOrderItem.LineNumber", Message: "line number must be positive"}
	}
	if i.BuyerItemCode == "" {
		return &ValidationError{Field: "EDIOrderItem.BuyerItemCode", Message: "buyer item code is required"}
	}
	if i.Quantity <= 0 {
		return &ValidationError{Field: "EDIOrderItem.Quantity", Message: "quantity must be positive"}
	}
	if i.UnitPrice < 0 {
		return &ValidationError{Field: "EDIOrderItem.UnitPrice", Message: "unit price cannot be negative"}
	}
	return nil
}

type EDIOrder struct {
	InterchangeSenderID     string
	InterchangeReceiverID   string
	InterchangeControlRef   string
	MessageRefNumber        string
	OrderNumber             string
	OrderDate               time.Time
	Currency                string
	CurrencyQualifier       string
	Buyer                   Address
	Seller                  Address
	Delivery                Address
	Invoice                 Address
	DeliveryDate            time.Time
	DeliveryDateQualifier   string
	DeliveryTerms           string
	DeliveryTermsCode       string
	PaymentTerms            string
	PaymentTermsCode        string
	TransportMode           string
	TransportModeCode       string
	Items                   []EDIOrderItem
	TotalAmount             float64
	TotalLines              int
	TotalQuantity           float64
	TestIndicator           int
	MessageVersion          string
	MessageRelease          string
	ResponsibleAgency       string
	AssociationCode         string
	SyntaxIdentifier        string
	SyntaxVersion           string
}

func (o EDIOrder) Validate() error {
	if o.InterchangeSenderID == "" {
		return &ValidationError{Field: "EDIOrder.InterchangeSenderID", Message: "interchange sender ID is required"}
	}
	if o.InterchangeReceiverID == "" {
		return &ValidationError{Field: "EDIOrder.InterchangeReceiverID", Message: "interchange receiver ID is required"}
	}
	if o.InterchangeControlRef == "" {
		return &ValidationError{Field: "EDIOrder.InterchangeControlRef", Message: "interchange control reference is required"}
	}
	if o.MessageRefNumber == "" {
		return &ValidationError{Field: "EDIOrder.MessageRefNumber", Message: "message reference number is required"}
	}
	if o.OrderNumber == "" {
		return &ValidationError{Field: "EDIOrder.OrderNumber", Message: "order number is required"}
	}
	if o.OrderDate.IsZero() {
		return &ValidationError{Field: "EDIOrder.OrderDate", Message: "order date is required"}
	}
	if err := o.Buyer.Validate(); err != nil {
		return fmt.Errorf("buyer validation failed: %w", err)
	}
	if err := o.Seller.Validate(); err != nil {
		return fmt.Errorf("seller validation failed: %w", err)
	}
	if o.Delivery.Name != "" {
		if err := o.Delivery.Validate(); err != nil {
			return fmt.Errorf("delivery validation failed: %w", err)
		}
	}
	if len(o.Items) == 0 {
		return &ValidationError{Field: "EDIOrder.Items", Message: "at least one item is required"}
	}
	for i, item := range o.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("item at index %d validation failed: %w", i, err)
		}
	}
	if o.TotalLines != len(o.Items) {
		return &ValidationError{Field: "EDIOrder.TotalLines", Message: "total lines does not match number of items"}
	}
	return nil
}

type SegmentBuilder interface {
	BuildUNB(order EDIOrder) (EDISegment, error)
	BuildUNH(order EDIOrder) (EDISegment, error)
	BuildBGM(order EDIOrder) (EDISegment, error)
	BuildDTM(date time.Time, qualifier string) (EDISegment, error)
	BuildCUX(order EDIOrder) (EDISegment, error)
	BuildNAD(partyQualifier string, address Address) (EDISegment, error)
	BuildTOD(order EDIOrder) (EDISegment, error)
	BuildPAT(order EDIOrder) (EDISegment, error)
	BuildTDT(order EDIOrder) (EDISegment, error)
	BuildLIN(item EDIOrderItem) (EDISegment, error)
	BuildIMD(item EDIOrderItem) (EDISegment, error)
	BuildQTY(item EDIOrderItem) (EDISegment, error)
	BuildPRI(item EDIOrderItem) (EDISegment, error)
	BuildMOA(item EDIOrderItem) (EDISegment, error)
	BuildCNT(order EDIOrder) (EDISegment, error)
	BuildMOATotal(order EDIOrder) (EDISegment, error)
	BuildUNT(order EDIOrder, segmentCount int) (EDISegment, error)
	BuildUNZ(order EDIOrder, messageCount int) (EDISegment, error)
}

type EDIFACTOrderGenerator struct {
	mu                 sync.RWMutex
	segmentTerminator  string
	elementSeparator   string
	componentSeparator string
	decimalMark        string
	releaseCharacter   string
	segmentBuilder     SegmentBuilder
}

type DefaultSegmentBuilder struct {
	generator *EDIFACTOrderGenerator
}

func NewEDIFACTOrderGenerator() *EDIFACTOrderGenerator {
	g := &EDIFACTOrderGenerator{
		segmentTerminator:  "'",
		elementSeparator:   "+",
		componentSeparator: ":",
		decimalMark:        ".",
		releaseCharacter:   "?",
	}
	g.segmentBuilder = &DefaultSegmentBuilder{generator: g}
	return g
}

func (g *EDIFACTOrderGenerator) WithCustomSeparators(terminator, element, component, decimal, release string) *EDIFACTOrderGenerator {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.segmentTerminator = terminator
	g.elementSeparator = element
	g.componentSeparator = component
	g.decimalMark = decimal
	g.releaseCharacter = release
	return g
}

func (g *EDIFACTOrderGenerator) WithSegmentBuilder(builder SegmentBuilder) *EDIFACTOrderGenerator {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.segmentBuilder = builder
	return g
}

func (g *EDIFACTOrderGenerator) Generate(order EDIOrder) (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	if err := order.Validate(); err != nil {
		return "", fmt.Errorf("order validation failed: %w", err)
	}
	
	var segments []EDISegment
	var err error
	
	unb, err := g.segmentBuilder.BuildUNB(order)
	if err != nil {
		return "", fmt.Errorf("failed to build UNB: %w", err)
	}
	segments = append(segments, unb)
	
	unh, err := g.segmentBuilder.BuildUNH(order)
	if err != nil {
		return "", fmt.Errorf("failed to build UNH: %w", err)
	}
	segments = append(segments, unh)
	
	bgm, err := g.segmentBuilder.BuildBGM(order)
	if err != nil {
		return "", fmt.Errorf("failed to build BGM: %w", err)
	}
	segments = append(segments, bgm)
	
	dtm, err := g.segmentBuilder.BuildDTM(order.OrderDate, QualifierDocumentDate)
	if err != nil {
		return "", fmt.Errorf("failed to build DTM: %w", err)
	}
	segments = append(segments, dtm)
	
	if !order.DeliveryDate.IsZero() {
		qualifier := QualifierDeliveryDate
		if order.DeliveryDateQualifier != "" {
			qualifier = order.DeliveryDateQualifier
		}
		deliveryDTM, err := g.segmentBuilder.BuildDTM(order.DeliveryDate, qualifier)
		if err != nil {
			return "", fmt.Errorf("failed to build delivery DTM: %w", err)
		}
		segments = append(segments, deliveryDTM)
	}
	
	if order.Currency != "" {
		cux, err := g.segmentBuilder.BuildCUX(order)
		if err != nil {
			return "", fmt.Errorf("failed to build CUX: %w", err)
		}
		segments = append(segments, cux)
	}
	
	if order.Buyer.Name != "" {
		buyerNAD, err := g.segmentBuilder.BuildNAD(PartyBuyer, order.Buyer)
		if err != nil {
			return "", fmt.Errorf("failed to build buyer NAD: %w", err)
		}
		segments = append(segments, buyerNAD)
	}
	
	if order.Seller.Name != "" {
		sellerNAD, err := g.segmentBuilder.BuildNAD(PartySeller, order.Seller)
		if err != nil {
			return "", fmt.Errorf("failed to build seller NAD: %w", err)
		}
		segments = append(segments, sellerNAD)
	}
	
	if order.Delivery.Name != "" {
		deliveryNAD, err := g.segmentBuilder.BuildNAD(PartyDelivery, order.Delivery)
		if err != nil {
			return "", fmt.Errorf("failed to build delivery NAD: %w", err)
		}
		segments = append(segments, deliveryNAD)
	}
	
	if order.Invoice.Name != "" {
		invoiceNAD, err := g.segmentBuilder.BuildNAD(PartyInvoice, order.Invoice)
		if err != nil {
			return "", fmt.Errorf("failed to build invoice NAD: %w", err)
		}
		segments = append(segments, invoiceNAD)
	}
	
	if order.DeliveryTerms != "" || order.DeliveryTermsCode != "" {
		tod, err := g.segmentBuilder.BuildTOD(order)
		if err != nil {
			return "", fmt.Errorf("failed to build TOD: %w", err)
		}
		segments = append(segments, tod)
	}
	
	if order.PaymentTerms != "" || order.PaymentTermsCode != "" {
		pat, err := g.segmentBuilder.BuildPAT(order)
		if err != nil {
			return "", fmt.Errorf("failed to build PAT: %w", err)
		}
		segments = append(segments, pat)
	}
	
	if order.TransportMode != "" || order.TransportModeCode != "" {
		tdt, err := g.segmentBuilder.BuildTDT(order)
		if err != nil {
			return "", fmt.Errorf("failed to build TDT: %w", err)
		}
		segments = append(segments, tdt)
	}
	
	for _, item := range order.Items {
		lin, err := g.segmentBuilder.BuildLIN(item)
		if err != nil {
			return "", fmt.Errorf("failed to build LIN: %w", err)
		}
		segments = append(segments, lin)
		
		imd, err := g.segmentBuilder.BuildIMD(item)
		if err != nil {
			return "", fmt.Errorf("failed to build IMD: %w", err)
		}
		segments = append(segments, imd)
		
		qty, err := g.segmentBuilder.BuildQTY(item)
		if err != nil {
			return "", fmt.Errorf("failed to build QTY: %w", err)
		}
		segments = append(segments, qty)
		
		pri, err := g.segmentBuilder.BuildPRI(item)
		if err != nil {
			return "", fmt.Errorf("failed to build PRI: %w", err)
		}
		segments = append(segments, pri)
		
		moa, err := g.segmentBuilder.BuildMOA(item)
		if err != nil {
			return "", fmt.Errorf("failed to build MOA: %w", err)
		}
		segments = append(segments, moa)
		
		if !item.DeliveryDate.IsZero() {
			itemDTM, err := g.segmentBuilder.BuildDTM(item.DeliveryDate, QualifierLineDeliveryDate)
			if err != nil {
				return "", fmt.Errorf("failed to build item DTM: %w", err)
			}
			segments = append(segments, itemDTM)
		}
	}
	
	segments = append(segments, EDISegment{Tag: SegmentTagUNS, Elements: []string{"S"}})
	
	cnt, err := g.segmentBuilder.BuildCNT(order)
	if err != nil {
		return "", fmt.Errorf("failed to build CNT: %w", err)
	}
	segments = append(segments, cnt)
	
	moaTotal, err := g.segmentBuilder.BuildMOATotal(order)
	if err != nil {
		return "", fmt.Errorf("failed to build MOA total: %w", err)
	}
	segments = append(segments, moaTotal)
	
	segmentCount := g.countSegmentsBetweenUNHAndUNT(segments)
	unt, err := g.segmentBuilder.BuildUNT(order, segmentCount)
	if err != nil {
		return "", fmt.Errorf("failed to build UNT: %w", err)
	}
	segments = append(segments, unt)
	
	messageCount := g.countMessages(segments)
	unz, err := g.segmentBuilder.BuildUNZ(order, messageCount)
	if err != nil {
		return "", fmt.Errorf("failed to build UNZ: %w", err)
	}
	segments = append(segments, unz)

	var result strings.Builder
	for _, seg := range segments {
		result.WriteString(seg.String(g.elementSeparator, g.segmentTerminator))
		result.WriteString("\n")
	}
	
	return result.String(), nil
}

func (g *EDIFACTOrderGenerator) countSegmentsBetweenUNHAndUNT(segments []EDISegment) int {
	count := 0
	foundUNH := false
	for _, seg := range segments {
		if seg.Tag == SegmentTagUNH {
			foundUNH = true
			count = 1
		} else if seg.Tag == SegmentTagUNT {
			break
		} else if foundUNH {
			count++
		}
	}
	return count
}

func (g *EDIFACTOrderGenerator) countMessages(segments []EDISegment) int {
	count := 0
	for _, seg := range segments {
		if seg.Tag == SegmentTagUNH {
			count++
		}
	}
	return count
}

func (b *DefaultSegmentBuilder) BuildUNB(order EDIOrder) (EDISegment, error) {
	date := order.OrderDate.Format(DateFormatYYMMDD)
	time := order.OrderDate.Format(DateFormatHHMM)
	
	syntaxID := "UNOA"
	syntaxVersion := "2"
	if order.SyntaxIdentifier != "" {
		syntaxID = order.SyntaxIdentifier
	}
	if order.SyntaxVersion != "" {
		syntaxVersion = order.SyntaxVersion
	}
	
	testIndicator := ""
	if order.TestIndicator == 1 {
		testIndicator = "1"
	}
	
	return EDISegment{
		Tag: SegmentTagUNB,
		Elements: []string{
			fmt.Sprintf("%s:%s", syntaxID, syntaxVersion),
			order.InterchangeSenderID,
			order.InterchangeReceiverID,
			date,
			time,
			order.InterchangeControlRef,
			"",
			"",
			testIndicator,
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildUNH(order EDIOrder) (EDISegment, error) {
	messageVersion := "D"
	messageRelease := "96A"
	responsibleAgency := "UN"
	associationCode := "EAN008"
	
	if order.MessageVersion != "" {
		messageVersion = order.MessageVersion
	}
	if order.MessageRelease != "" {
		messageRelease = order.MessageRelease
	}
	if order.ResponsibleAgency != "" {
		responsibleAgency = order.ResponsibleAgency
	}
	if order.AssociationCode != "" {
		associationCode = order.AssociationCode
	}
	
	return EDISegment{
		Tag: SegmentTagUNH,
		Elements: []string{
			order.MessageRefNumber,
			fmt.Sprintf("ORDERS:%s:%s:%s:%s", messageVersion, messageRelease, responsibleAgency, associationCode),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildBGM(order EDIOrder) (EDISegment, error) {
	return EDISegment{
		Tag: SegmentTagBGM,
		Elements: []string{
			CodeOrder,
			order.OrderNumber,
			CodeOriginal,
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildDTM(date time.Time, qualifier string) (EDISegment, error) {
	formattedDate := date.Format(DateFormatCCYYMMDD)
	return EDISegment{
		Tag: SegmentTagDTM,
		Elements: []string{
			fmt.Sprintf("%s:%s:102", qualifier, formattedDate),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildCUX(order EDIOrder) (EDISegment, error) {
	qualifier := CurrencyReference
	if order.CurrencyQualifier != "" {
		qualifier = order.CurrencyQualifier
	}
	
	return EDISegment{
		Tag: SegmentTagCUX,
		Elements: []string{
			fmt.Sprintf("%s:%s:9", qualifier, order.Currency),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildNAD(partyQualifier string, address Address) (EDISegment, error) {
	elements := []string{partyQualifier}
	
	idType := IDTypeBuyer
	if address.IDType != "" {
		idType = address.IDType
	}
	
	if address.ID != "" {
		elements = append(elements, fmt.Sprintf("%s::%s", address.ID, idType))
	} else {
		elements = append(elements, "")
	}
	
	addrStr := strings.Join(address.Lines, b.generator.componentSeparator)
	elements = append(elements, addrStr, "", address.Name)
	
	return EDISegment{Tag: SegmentTagNAD, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildTOD(order EDIOrder) (EDISegment, error) {
	elements := []string{"3", ""}
	
	if order.DeliveryTermsCode != "" {
		elements = append(elements, fmt.Sprintf("::%s", order.DeliveryTermsCode))
	} else {
		elements = append(elements, fmt.Sprintf("::%s", order.DeliveryTerms))
	}
	
	return EDISegment{Tag: SegmentTagTOD, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildPAT(order EDIOrder) (EDISegment, error) {
	elements := []string{"1", ""}
	
	if order.PaymentTermsCode != "" {
		elements = append(elements, order.PaymentTermsCode)
	} else {
		elements = append(elements, order.PaymentTerms)
	}
	
	return EDISegment{Tag: SegmentTagPAT, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildTDT(order EDIOrder) (EDISegment, error) {
	elements := []string{"20", "1", ""}
	
	if order.TransportModeCode != "" {
		elements = append(elements, order.TransportModeCode)
	} else {
		elements = append(elements, order.TransportMode)
	}
	
	return EDISegment{Tag: SegmentTagTDT, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildLIN(item EDIOrderItem) (EDISegment, error) {
	elements := []string{
		strconv.Itoa(item.LineNumber),
		"",
		fmt.Sprintf("%s:EN", item.BuyerItemCode),
		"",
	}
	
	if item.SupplierItemCode != "" {
		elements = append(elements, fmt.Sprintf("%s:SA", item.SupplierItemCode))
	} else {
		elements = append(elements, "")
	}
	
	return EDISegment{Tag: SegmentTagLIN, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildIMD(item EDIOrderItem) (EDISegment, error) {
	return EDISegment{
		Tag: SegmentTagIMD,
		Elements: []string{
			"F",
			"",
			"",
			fmt.Sprintf(":::%s", item.Description),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildQTY(item EDIOrderItem) (EDISegment, error) {
	uom := item.UnitOfMeasure
	if uom == "" {
		uom = "PCE"
	}
	
	quantityStr := strconv.FormatFloat(item.Quantity, 'f', -1, 64)
	
	return EDISegment{
		Tag: SegmentTagQTY,
		Elements: []string{
			fmt.Sprintf("%s:%s:%s", QuantityOrdered, quantityStr, uom),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildPRI(item EDIOrderItem) (EDISegment, error) {
	priceStr := strconv.FormatFloat(item.UnitPrice, 'f', -1, 64)
	
	return EDISegment{
		Tag: SegmentTagPRI,
		Elements: []string{
			fmt.Sprintf("%s:%s", PriceNet, priceStr),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildMOA(item EDIOrderItem) (EDISegment, error) {
	amountStr := strconv.FormatFloat(item.Amount, 'f', -1, 64)
	
	return EDISegment{
		Tag: SegmentTagMOA,
		Elements: []string{
			fmt.Sprintf("%s:%s", AmountLine, amountStr),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildCNT(order EDIOrder) (EDISegment, error) {
	return EDISegment{
		Tag: SegmentTagCNT,
		Elements: []string{
			fmt.Sprintf("%s:%d", ControlTotalLines, order.TotalLines),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildMOATotal(order EDIOrder) (EDISegment, error) {
	amountStr := strconv.FormatFloat(order.TotalAmount, 'f', -1, 64)
	
	return EDISegment{
		Tag: SegmentTagMOA,
		Elements: []string{
			fmt.Sprintf("%s:%s", AmountTotal, amountStr),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildUNT(order EDIOrder, segmentCount int) (EDISegment, error) {
	return EDISegment{
		Tag: SegmentTagUNT,
		Elements: []string{
			strconv.Itoa(segmentCount),
			order.MessageRefNumber,
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildUNZ(order EDIOrder, messageCount int) (EDISegment, error) {
	return EDISegment{
		Tag: SegmentTagUNZ,
		Elements: []string{
			strconv.Itoa(messageCount),
			order.InterchangeControlRef,
		},
	}, nil
}

type EDIWriter struct {
	outputDir string
	mu        sync.Mutex
}

func NewEDIWriter(outputDir string) *EDIWriter {
	return &EDIWriter{outputDir: outputDir}
}

func (w *EDIWriter) WriteOrder(ctx context.Context, order EDIOrder, content string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if err := os.MkdirAll(w.outputDir, DirPerms); err != nil {
		return "", fmt.Errorf("%w: failed to create directory: %v", ErrFileWrite, err)
	}
	
	timestamp := time.Now().Format("20060102_150405")
	safeOrderNumber := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, order.OrderNumber)
	
	filename := filepath.Join(w.outputDir, fmt.Sprintf("ORDER_%s_%s.edi", safeOrderNumber, timestamp))
	
	if !strings.HasPrefix(filepath.Clean(filename), filepath.Clean(w.outputDir)) {
		return "", fmt.Errorf("%w: path traversal detected", ErrFileWrite)
	}
	
	if err := os.WriteFile(filename, []byte(content), FilePerms); err != nil {
		return "", fmt.Errorf("%w: failed to write file: %v", ErrFileWrite, err)
	}
	
	return filename, nil
}

func (w *EDIWriter) WriteOrderWithPrefix(ctx context.Context, order EDIOrder, content string, prefix string) (string, error) {
	safePrefix := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, prefix)
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if err := os.MkdirAll(w.outputDir, DirPerms); err != nil {
		return "", fmt.Errorf("%w: failed to create directory: %v", ErrFileWrite, err)
	}
	
	timestamp := time.Now().Format("20060102_150405")
	safeOrderNumber := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, order.OrderNumber)
	
	filename := filepath.Join(w.outputDir, fmt.Sprintf("%s_%s_%s.edi", safePrefix, safeOrderNumber, timestamp))
	
	if !strings.HasPrefix(filepath.Clean(filename), filepath.Clean(w.outputDir)) {
		return "", fmt.Errorf("%w: path traversal detected", ErrFileWrite)
	}
	
	if err := os.WriteFile(filename, []byte(content), FilePerms); err != nil {
		return "", fmt.Errorf("%w: failed to write file: %v", ErrFileWrite, err)
	}
	
	return filename, nil
}

func main() {
	generator := NewEDIFACTOrderGenerator()
	writer := NewEDIWriter("./edi_output")
	
	order := EDIOrder{
		InterchangeSenderID:   "SENDERID",
		InterchangeReceiverID: "RECEIVERID",
		InterchangeControlRef: "12345",
		MessageRefNumber:      "12345",
		OrderNumber:           "PO-2024-001",
		OrderDate:             time.Now(),
		Currency:              "USD",
		CurrencyQualifier:     "2",
		
		Buyer: Address{
			Name:   "Acme Corporation",
			Lines:  []string{"123 Main St", "Suite 100", "New York", "NY 10001"},
			ID:     "BUYER001",
			IDType: "9",
		},
		
		Seller: Address{
			Name:   "Supplier Inc",
			Lines:  []string{"456 Supply Ave", "Industrial Park", "Chicago", "IL 60601"},
			ID:     "SUP001",
			IDType: "9",
		},
		
		Delivery: Address{
			Name:  "Acme Warehouse",
			Lines: []string{"789 Distribution Blvd", "Warehouse 5", "Newark", "NJ 07101"},
		},
		
		DeliveryDate:      time.Now().AddDate(0, 0, 7),
		DeliveryTerms:     "CFR",
		PaymentTerms:      "Net 30",
		
		Items: []EDIOrderItem{
			{
				LineNumber:      1,
				BuyerItemCode:   "ITEM001",
				SupplierItemCode: "SUP-001",
				Quantity:        10,
				UnitPrice:       25.50,
				UnitOfMeasure:   "PCE",
				Description:     "Widget Type A",
				TaxRate:         10.0,
				Amount:          255.00,
			},
			{
				LineNumber:      2,
				BuyerItemCode:   "ITEM002",
				SupplierItemCode: "SUP-002",
				Quantity:        5,
				UnitPrice:       99.99,
				UnitOfMeasure:   "PCE",
				Description:     "Gadget Pro",
				TaxRate:         10.0,
				Amount:          499.95,
			},
		},
		
		TotalAmount:   754.95,
		TotalLines:    2,
		TotalQuantity: 15,
		TestIndicator: 1,
		
		MessageVersion:    "D",
		MessageRelease:    "96A",
		ResponsibleAgency: "UN",
		AssociationCode:   "EAN008",
		SyntaxIdentifier:  "UNOA",
		SyntaxVersion:     "2",
	}
	
	ctx := context.Background()
	
	ediMessage, err := generator.Generate(order)
	if err != nil {
		fmt.Printf("Error generating EDIFACT message: %v\n", err)
		return
	}
	
	filename, err := writer.WriteOrder(ctx, order, ediMessage)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("EDIFACT order generated successfully: %s\n", filename)
	fmt.Println("\nEDIFACT Message Content:")
	fmt.Println(ediMessage)
}
