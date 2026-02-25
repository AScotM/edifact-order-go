package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	MaxSegmentLength = 1000
)

var (
	ErrInvalidOrder = errors.New("invalid order data")
	ErrMissingField = errors.New("required field missing")
	ErrFileWrite = errors.New("failed to write file")
	ErrInvalidSeparator = errors.New("invalid separator character")
	ErrSegmentTooLong = errors.New("segment exceeds maximum length")
	ErrContextCancelled = errors.New("context cancelled")
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

func (s EDISegment) String(separator string, terminator string, releaseChar string) (string, error) {
	var escapedElements []string
	for _, elem := range s.Elements {
		escaped := strings.ReplaceAll(elem, separator, releaseChar+separator)
		escaped = strings.ReplaceAll(escaped, terminator, releaseChar+terminator)
		escaped = strings.ReplaceAll(escaped, releaseChar, releaseChar+releaseChar)
		escapedElements = append(escapedElements, escaped)
	}
	
	result := s.Tag + separator + strings.Join(escapedElements, separator) + terminator
	
	if len(result) > MaxSegmentLength {
		return "", ErrSegmentTooLong
	}
	
	return result, nil
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
	for i, line := range a.Lines {
		if len(line) > 35 {
			return &ValidationError{Field: fmt.Sprintf("Address.Lines[%d]", i), Message: "address line exceeds 35 characters"}
		}
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
	if len(i.BuyerItemCode) > 35 {
		return &ValidationError{Field: "EDIOrderItem.BuyerItemCode", Message: "buyer item code exceeds 35 characters"}
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
	if len(o.InterchangeSenderID) > 35 {
		return &ValidationError{Field: "EDIOrder.InterchangeSenderID", Message: "interchange sender ID exceeds 35 characters"}
	}
	if o.InterchangeReceiverID == "" {
		return &ValidationError{Field: "EDIOrder.InterchangeReceiverID", Message: "interchange receiver ID is required"}
	}
	if len(o.InterchangeReceiverID) > 35 {
		return &ValidationError{Field: "EDIOrder.InterchangeReceiverID", Message: "interchange receiver ID exceeds 35 characters"}
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
	if len(o.OrderNumber) > 35 {
		return &ValidationError{Field: "EDIOrder.OrderNumber", Message: "order number exceeds 35 characters"}
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
	if len(o.Items) > 999999 {
		return &ValidationError{Field: "EDIOrder.Items", Message: "too many items"}
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
	BuildUNB(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildUNH(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildBGM(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildDTM(ctx context.Context, date time.Time, qualifier string) (EDISegment, error)
	BuildCUX(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildNAD(ctx context.Context, partyQualifier string, address Address) (EDISegment, error)
	BuildTOD(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildPAT(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildTDT(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildLIN(ctx context.Context, item EDIOrderItem) (EDISegment, error)
	BuildIMD(ctx context.Context, item EDIOrderItem) (EDISegment, error)
	BuildQTY(ctx context.Context, item EDIOrderItem) (EDISegment, error)
	BuildPRI(ctx context.Context, item EDIOrderItem) (EDISegment, error)
	BuildMOA(ctx context.Context, item EDIOrderItem) (EDISegment, error)
	BuildCNT(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildMOATotal(ctx context.Context, order EDIOrder) (EDISegment, error)
	BuildUNT(ctx context.Context, order EDIOrder, segmentCount int) (EDISegment, error)
	BuildUNZ(ctx context.Context, order EDIOrder, messageCount int) (EDISegment, error)
}

type EDIFACTOrderGenerator struct {
	segmentTerminator  string
	elementSeparator   string
	componentSeparator string
	decimalMark        string
	releaseCharacter   string
	segmentBuilder     SegmentBuilder
	pool               sync.Pool
}

type DefaultSegmentBuilder struct {
	generator *EDIFACTOrderGenerator
}

func NewEDIFACTOrderGenerator() (*EDIFACTOrderGenerator, error) {
	g := &EDIFACTOrderGenerator{
		segmentTerminator:  "'",
		elementSeparator:   "+",
		componentSeparator: ":",
		decimalMark:        ".",
		releaseCharacter:   "?",
		pool: sync.Pool{
			New: func() interface{} {
				return &strings.Builder{}
			},
		},
	}
	
	if err := g.validateSeparators(); err != nil {
		return nil, err
	}
	
	g.segmentBuilder = &DefaultSegmentBuilder{generator: g}
	return g, nil
}

func (g *EDIFACTOrderGenerator) validateSeparators() error {
	chars := map[rune]bool{
		rune(g.segmentTerminator[0]): true,
		rune(g.elementSeparator[0]): true,
		rune(g.componentSeparator[0]): true,
		rune(g.releaseCharacter[0]): true,
	}
	
	if len(chars) != 4 {
		return ErrInvalidSeparator
	}
	
	return nil
}

func (g *EDIFACTOrderGenerator) WithCustomSeparators(terminator, element, component, decimal, release string) (*EDIFACTOrderGenerator, error) {
	g.segmentTerminator = terminator
	g.elementSeparator = element
	g.componentSeparator = component
	g.decimalMark = decimal
	g.releaseCharacter = release
	
	if err := g.validateSeparators(); err != nil {
		return nil, err
	}
	
	return g, nil
}

func (g *EDIFACTOrderGenerator) WithSegmentBuilder(builder SegmentBuilder) *EDIFACTOrderGenerator {
	g.segmentBuilder = builder
	return g
}

func (g *EDIFACTOrderGenerator) Generate(ctx context.Context, order EDIOrder, writer io.Writer) error {
	select {
	case <-ctx.Done():
		return ErrContextCancelled
	default:
	}
	
	if err := order.Validate(); err != nil {
		return fmt.Errorf("order validation failed: %w", err)
	}
	
	segmentCount := 0
	foundUNH := false
	
	unb, err := g.segmentBuilder.BuildUNB(ctx, order)
	if err != nil {
		return fmt.Errorf("failed to build UNB: %w", err)
	}
	
	if err := g.writeSegment(unb, writer); err != nil {
		return err
	}
	
	unh, err := g.segmentBuilder.BuildUNH(ctx, order)
	if err != nil {
		return fmt.Errorf("failed to build UNH: %w", err)
	}
	
	if err := g.writeSegment(unh, writer); err != nil {
		return err
	}
	foundUNH = true
	segmentCount = 1
	
	bgm, err := g.segmentBuilder.BuildBGM(ctx, order)
	if err != nil {
		return fmt.Errorf("failed to build BGM: %w", err)
	}
	
	if err := g.writeSegment(bgm, writer); err != nil {
		return err
	}
	if foundUNH {
		segmentCount++
	}
	
	dtm, err := g.segmentBuilder.BuildDTM(ctx, order.OrderDate, QualifierDocumentDate)
	if err != nil {
		return fmt.Errorf("failed to build DTM: %w", err)
	}
	
	if err := g.writeSegment(dtm, writer); err != nil {
		return err
	}
	if foundUNH {
		segmentCount++
	}
	
	if !order.DeliveryDate.IsZero() {
		qualifier := QualifierDeliveryDate
		if order.DeliveryDateQualifier != "" {
			qualifier = order.DeliveryDateQualifier
		}
		deliveryDTM, err := g.segmentBuilder.BuildDTM(ctx, order.DeliveryDate, qualifier)
		if err != nil {
			return fmt.Errorf("failed to build delivery DTM: %w", err)
		}
		
		if err := g.writeSegment(deliveryDTM, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.Currency != "" {
		cux, err := g.segmentBuilder.BuildCUX(ctx, order)
		if err != nil {
			return fmt.Errorf("failed to build CUX: %w", err)
		}
		
		if err := g.writeSegment(cux, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.Buyer.Name != "" {
		buyerNAD, err := g.segmentBuilder.BuildNAD(ctx, PartyBuyer, order.Buyer)
		if err != nil {
			return fmt.Errorf("failed to build buyer NAD: %w", err)
		}
		
		if err := g.writeSegment(buyerNAD, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.Seller.Name != "" {
		sellerNAD, err := g.segmentBuilder.BuildNAD(ctx, PartySeller, order.Seller)
		if err != nil {
			return fmt.Errorf("failed to build seller NAD: %w", err)
		}
		
		if err := g.writeSegment(sellerNAD, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.Delivery.Name != "" {
		deliveryNAD, err := g.segmentBuilder.BuildNAD(ctx, PartyDelivery, order.Delivery)
		if err != nil {
			return fmt.Errorf("failed to build delivery NAD: %w", err)
		}
		
		if err := g.writeSegment(deliveryNAD, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.Invoice.Name != "" {
		invoiceNAD, err := g.segmentBuilder.BuildNAD(ctx, PartyInvoice, order.Invoice)
		if err != nil {
			return fmt.Errorf("failed to build invoice NAD: %w", err)
		}
		
		if err := g.writeSegment(invoiceNAD, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.DeliveryTerms != "" || order.DeliveryTermsCode != "" {
		tod, err := g.segmentBuilder.BuildTOD(ctx, order)
		if err != nil {
			return fmt.Errorf("failed to build TOD: %w", err)
		}
		
		if err := g.writeSegment(tod, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.PaymentTerms != "" || order.PaymentTermsCode != "" {
		pat, err := g.segmentBuilder.BuildPAT(ctx, order)
		if err != nil {
			return fmt.Errorf("failed to build PAT: %w", err)
		}
		
		if err := g.writeSegment(pat, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	if order.TransportMode != "" || order.TransportModeCode != "" {
		tdt, err := g.segmentBuilder.BuildTDT(ctx, order)
		if err != nil {
			return fmt.Errorf("failed to build TDT: %w", err)
		}
		
		if err := g.writeSegment(tdt, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
	}
	
	for _, item := range order.Items {
		select {
		case <-ctx.Done():
			return ErrContextCancelled
		default:
		}
		
		lin, err := g.segmentBuilder.BuildLIN(ctx, item)
		if err != nil {
			return fmt.Errorf("failed to build LIN: %w", err)
		}
		
		if err := g.writeSegment(lin, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
		
		imd, err := g.segmentBuilder.BuildIMD(ctx, item)
		if err != nil {
			return fmt.Errorf("failed to build IMD: %w", err)
		}
		
		if err := g.writeSegment(imd, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
		
		qty, err := g.segmentBuilder.BuildQTY(ctx, item)
		if err != nil {
			return fmt.Errorf("failed to build QTY: %w", err)
		}
		
		if err := g.writeSegment(qty, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
		
		pri, err := g.segmentBuilder.BuildPRI(ctx, item)
		if err != nil {
			return fmt.Errorf("failed to build PRI: %w", err)
		}
		
		if err := g.writeSegment(pri, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
		
		moa, err := g.segmentBuilder.BuildMOA(ctx, item)
		if err != nil {
			return fmt.Errorf("failed to build MOA: %w", err)
		}
		
		if err := g.writeSegment(moa, writer); err != nil {
			return err
		}
		if foundUNH {
			segmentCount++
		}
		
		if !item.DeliveryDate.IsZero() {
			itemDTM, err := g.segmentBuilder.BuildDTM(ctx, item.DeliveryDate, QualifierLineDeliveryDate)
			if err != nil {
				return fmt.Errorf("failed to build item DTM: %w", err)
			}
			
			if err := g.writeSegment(itemDTM, writer); err != nil {
				return err
			}
			if foundUNH {
				segmentCount++
			}
		}
	}
	
	uns := EDISegment{Tag: SegmentTagUNS, Elements: []string{"S"}}
	if err := g.writeSegment(uns, writer); err != nil {
		return err
	}
	if foundUNH {
		segmentCount++
	}
	
	cnt, err := g.segmentBuilder.BuildCNT(ctx, order)
	if err != nil {
		return fmt.Errorf("failed to build CNT: %w", err)
	}
	
	if err := g.writeSegment(cnt, writer); err != nil {
		return err
	}
	if foundUNH {
		segmentCount++
	}
	
	moaTotal, err := g.segmentBuilder.BuildMOATotal(ctx, order)
	if err != nil {
		return fmt.Errorf("failed to build MOA total: %w", err)
	}
	
	if err := g.writeSegment(moaTotal, writer); err != nil {
		return err
	}
	if foundUNH {
		segmentCount++
	}
	
	unt, err := g.segmentBuilder.BuildUNT(ctx, order, segmentCount)
	if err != nil {
		return fmt.Errorf("failed to build UNT: %w", err)
	}
	
	if err := g.writeSegment(unt, writer); err != nil {
		return err
	}
	
	messageCount := 1
	unz, err := g.segmentBuilder.BuildUNZ(ctx, order, messageCount)
	if err != nil {
		return fmt.Errorf("failed to build UNZ: %w", err)
	}
	
	if err := g.writeSegment(unz, writer); err != nil {
		return err
	}
	
	return nil
}

func (g *EDIFACTOrderGenerator) writeSegment(segment EDISegment, writer io.Writer) error {
	builder := g.pool.Get().(*strings.Builder)
	builder.Reset()
	defer g.pool.Put(builder)
	
	str, err := segment.String(g.elementSeparator, g.segmentTerminator, g.releaseCharacter)
	if err != nil {
		return err
	}
	
	builder.WriteString(str)
	builder.WriteString("\n")
	
	_, err = writer.Write([]byte(builder.String()))
	return err
}

func (b *DefaultSegmentBuilder) BuildUNB(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
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

func (b *DefaultSegmentBuilder) BuildUNH(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
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

func (b *DefaultSegmentBuilder) BuildBGM(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	return EDISegment{
		Tag: SegmentTagBGM,
		Elements: []string{
			CodeOrder,
			order.OrderNumber,
			CodeOriginal,
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildDTM(ctx context.Context, date time.Time, qualifier string) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	formattedDate := date.Format(DateFormatCCYYMMDD)
	return EDISegment{
		Tag: SegmentTagDTM,
		Elements: []string{
			fmt.Sprintf("%s:%s:102", qualifier, formattedDate),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildCUX(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
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

func (b *DefaultSegmentBuilder) BuildNAD(ctx context.Context, partyQualifier string, address Address) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
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

func (b *DefaultSegmentBuilder) BuildTOD(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	elements := []string{"3", ""}
	
	if order.DeliveryTermsCode != "" {
		elements = append(elements, fmt.Sprintf("::%s", order.DeliveryTermsCode))
	} else {
		elements = append(elements, fmt.Sprintf("::%s", order.DeliveryTerms))
	}
	
	return EDISegment{Tag: SegmentTagTOD, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildPAT(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	elements := []string{"1", ""}
	
	if order.PaymentTermsCode != "" {
		elements = append(elements, order.PaymentTermsCode)
	} else {
		elements = append(elements, order.PaymentTerms)
	}
	
	return EDISegment{Tag: SegmentTagPAT, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildTDT(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	elements := []string{"20", "1", ""}
	
	if order.TransportModeCode != "" {
		elements = append(elements, order.TransportModeCode)
	} else {
		elements = append(elements, order.TransportMode)
	}
	
	return EDISegment{Tag: SegmentTagTDT, Elements: elements}, nil
}

func (b *DefaultSegmentBuilder) BuildLIN(ctx context.Context, item EDIOrderItem) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
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

func (b *DefaultSegmentBuilder) BuildIMD(ctx context.Context, item EDIOrderItem) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
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

func (b *DefaultSegmentBuilder) BuildQTY(ctx context.Context, item EDIOrderItem) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	uom := item.UnitOfMeasure
	if uom == "" {
		uom = "PCE"
	}
	
	quantityStr := strconv.FormatFloat(item.Quantity, 'f', 2, 64)
	
	return EDISegment{
		Tag: SegmentTagQTY,
		Elements: []string{
			fmt.Sprintf("%s:%s:%s", QuantityOrdered, quantityStr, uom),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildPRI(ctx context.Context, item EDIOrderItem) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	priceStr := strconv.FormatFloat(item.UnitPrice, 'f', 2, 64)
	
	return EDISegment{
		Tag: SegmentTagPRI,
		Elements: []string{
			fmt.Sprintf("%s:%s", PriceNet, priceStr),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildMOA(ctx context.Context, item EDIOrderItem) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	amountStr := strconv.FormatFloat(item.Amount, 'f', 2, 64)
	
	return EDISegment{
		Tag: SegmentTagMOA,
		Elements: []string{
			fmt.Sprintf("%s:%s", AmountLine, amountStr),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildCNT(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	return EDISegment{
		Tag: SegmentTagCNT,
		Elements: []string{
			fmt.Sprintf("%s:%d", ControlTotalLines, order.TotalLines),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildMOATotal(ctx context.Context, order EDIOrder) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	amountStr := strconv.FormatFloat(order.TotalAmount, 'f', 2, 64)
	
	return EDISegment{
		Tag: SegmentTagMOA,
		Elements: []string{
			fmt.Sprintf("%s:%s", AmountTotal, amountStr),
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildUNT(ctx context.Context, order EDIOrder, segmentCount int) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
	return EDISegment{
		Tag: SegmentTagUNT,
		Elements: []string{
			strconv.Itoa(segmentCount),
			order.MessageRefNumber,
		},
	}, nil
}

func (b *DefaultSegmentBuilder) BuildUNZ(ctx context.Context, order EDIOrder, messageCount int) (EDISegment, error) {
	select {
	case <-ctx.Done():
		return EDISegment{}, ErrContextCancelled
	default:
	}
	
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
	
	if err := os.MkdirAll(w.outputDir, DirPerms); err != nil {
		return "", fmt.Errorf("%w: failed to create directory: %v", ErrFileWrite, err)
	}
	
	timestamp := time.Now().Format("20060102_150405")
	safeOrderNumber := sanitizeFilename(order.OrderNumber)
	
	filename := filepath.Join(w.outputDir, fmt.Sprintf("ORDER_%s_%s.edi", safeOrderNumber, timestamp))
	
	if !isPathSafe(w.outputDir, filename) {
		return "", fmt.Errorf("%w: path traversal detected", ErrFileWrite)
	}
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, FilePerms)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create file: %v", ErrFileWrite, err)
	}
	defer file.Close()
	
	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("%w: failed to write content: %v", ErrFileWrite, err)
	}
	
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("%w: failed to sync file: %v", ErrFileWrite, err)
	}
	
	return filename, nil
}

func sanitizeFilename(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

func isPathSafe(base, path string) bool {
	cleanBase := filepath.Clean(base)
	cleanPath := filepath.Clean(path)
	return strings.HasPrefix(cleanPath, cleanBase+string(os.PathSeparator)) || cleanPath == cleanBase
}

func main() {
	ctx := context.Background()
	
	generator, err := NewEDIFACTOrderGenerator()
	if err != nil {
		fmt.Printf("Error creating generator: %v\n", err)
		return
	}
	
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
	
	var buffer strings.Builder
	
	err = generator.Generate(ctx, order, &buffer)
	if err != nil {
		fmt.Printf("Error generating EDIFACT message: %v\n", err)
		return
	}
	
	ediMessage := buffer.String()
	
	filename, err := writer.WriteOrder(ctx, order, ediMessage)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("EDIFACT order generated successfully: %s\n", filename)
	fmt.Println("\nEDIFACT Message Content:")
	fmt.Println(ediMessage)
}
