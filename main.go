package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type EDISegment struct {
	Tag      string
	Elements []string
}

func (s EDISegment) String() string {
	return s.Tag + "+" + strings.Join(s.Elements, "+") + "'"
}

type Address struct {
	Name    string
	Lines   []string
	ID      string
	IDType  string
}

type OrderLineItem struct {
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
	Items                   []OrderLineItem
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

type EDIFACTOrderGenerator struct {
	SegmentTerminator  string
	ElementSeparator   string
	ComponentSeparator string
	DecimalMark        string
	ReleaseCharacter   string
}

func NewEDIFACTOrderGenerator() *EDIFACTOrderGenerator {
	return &EDIFACTOrderGenerator{
		SegmentTerminator:  "'",
		ElementSeparator:   "+",
		ComponentSeparator: ":",
		DecimalMark:        ".",
		ReleaseCharacter:   "?",
	}
}

func (g *EDIFACTOrderGenerator) Generate(order EDIOrder) string {
	var segments []EDISegment

	segments = append(segments, g.createUNB(order))
	segments = append(segments, g.createUNH(order))
	segments = append(segments, g.createBGM(order))
	segments = append(segments, g.createDTM(order.OrderDate, "137"))
	
	if order.DeliveryDate != (time.Time{}) {
		qualifier := "2"
		if order.DeliveryDateQualifier != "" {
			qualifier = order.DeliveryDateQualifier
		}
		segments = append(segments, g.createDTM(order.DeliveryDate, qualifier))
	}
	
	if order.Currency != "" {
		segments = append(segments, g.createCUX(order))
	}
	
	if order.Buyer.Name != "" {
		segments = append(segments, g.createNAD("BY", order.Buyer))
	}
	
	if order.Seller.Name != "" {
		segments = append(segments, g.createNAD("SE", order.Seller))
	}
	
	if order.Delivery.Name != "" {
		segments = append(segments, g.createNAD("DP", order.Delivery))
	}
	
	if order.Invoice.Name != "" {
		segments = append(segments, g.createNAD("IV", order.Invoice))
	}
	
	if order.DeliveryTerms != "" || order.DeliveryTermsCode != "" {
		segments = append(segments, g.createTOD(order))
	}
	
	if order.PaymentTerms != "" || order.PaymentTermsCode != "" {
		segments = append(segments, g.createPAT(order))
	}
	
	if order.TransportMode != "" || order.TransportModeCode != "" {
		segments = append(segments, g.createTDT(order))
	}
	
	for _, item := range order.Items {
		segments = append(segments, g.createLIN(item))
		segments = append(segments, g.createIMD(item))
		segments = append(segments, g.createQTY(item))
		segments = append(segments, g.createPRI(item))
		segments = append(segments, g.createMOA(item))
		if !item.DeliveryDate.IsZero() {
			segments = append(segments, g.createDTM(item.DeliveryDate, "64"))
		}
	}
	
	segments = append(segments, EDISegment{Tag: "UNS", Elements: []string{"S"}})
	segments = append(segments, g.createCNT(order))
	segments = append(segments, g.createMOATotal(order))
	segments = append(segments, g.createUNT(order, segments))
	segments = append(segments, g.createUNZ(order, segments))

	var result strings.Builder
	for _, seg := range segments {
		result.WriteString(seg.String())
		result.WriteString("\n")
	}
	
	return result.String()
}

func (g *EDIFACTOrderGenerator) createUNB(order EDIOrder) EDISegment {
	date := order.OrderDate.Format("060102")
	time := order.OrderDate.Format("1504")
	
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
		Tag: "UNB",
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
	}
}

func (g *EDIFACTOrderGenerator) createUNH(order EDIOrder) EDISegment {
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
		Tag: "UNH",
		Elements: []string{
			order.MessageRefNumber,
			fmt.Sprintf("ORDERS:%s:%s:%s:%s", messageVersion, messageRelease, responsibleAgency, associationCode),
		},
	}
}

func (g *EDIFACTOrderGenerator) createBGM(order EDIOrder) EDISegment {
	return EDISegment{
		Tag: "BGM",
		Elements: []string{
			"220",
			order.OrderNumber,
			"9",
		},
	}
}

func (g *EDIFACTOrderGenerator) createDTM(date time.Time, qualifier string) EDISegment {
	formattedDate := date.Format("20060102")
	return EDISegment{
		Tag: "DTM",
		Elements: []string{
			fmt.Sprintf("%s:%s:102", qualifier, formattedDate),
		},
	}
}

func (g *EDIFACTOrderGenerator) createCUX(order EDIOrder) EDISegment {
	qualifier := "2"
	if order.CurrencyQualifier != "" {
		qualifier = order.CurrencyQualifier
	}
	
	return EDISegment{
		Tag: "CUX",
		Elements: []string{
			fmt.Sprintf("%s:%s:9", qualifier, order.Currency),
		},
	}
}

func (g *EDIFACTOrderGenerator) createNAD(partyQualifier string, address Address) EDISegment {
	elements := []string{partyQualifier}
	
	idType := "9"
	if address.IDType != "" {
		idType = address.IDType
	}
	
	if address.ID != "" {
		elements = append(elements, fmt.Sprintf("%s::%s", address.ID, idType))
	} else {
		elements = append(elements, "")
	}
	
	addrStr := strings.Join(address.Lines, g.ComponentSeparator)
	elements = append(elements, addrStr, "", address.Name)
	
	return EDISegment{Tag: "NAD", Elements: elements}
}

func (g *EDIFACTOrderGenerator) createTOD(order EDIOrder) EDISegment {
	elements := []string{"3", ""}
	
	if order.DeliveryTermsCode != "" {
		elements = append(elements, fmt.Sprintf("::%s", order.DeliveryTermsCode))
	} else {
		elements = append(elements, fmt.Sprintf("::%s", order.DeliveryTerms))
	}
	
	return EDISegment{Tag: "TOD", Elements: elements}
}

func (g *EDIFACTOrderGenerator) createPAT(order EDIOrder) EDISegment {
	elements := []string{"1", ""}
	
	if order.PaymentTermsCode != "" {
		elements = append(elements, order.PaymentTermsCode)
	} else {
		elements = append(elements, order.PaymentTerms)
	}
	
	return EDISegment{Tag: "PAT", Elements: elements}
}

func (g *EDIFACTOrderGenerator) createTDT(order EDIOrder) EDISegment {
	elements := []string{"20", "1", ""}
	
	if order.TransportModeCode != "" {
		elements = append(elements, order.TransportModeCode)
	} else {
		elements = append(elements, order.TransportMode)
	}
	
	return EDISegment{Tag: "TDT", Elements: elements}
}

func (g *EDIFACTOrderGenerator) createLIN(item OrderLineItem) EDISegment {
	elements := []string{
		fmt.Sprintf("%d", item.LineNumber),
		"",
		fmt.Sprintf("%s:EN", item.BuyerItemCode),
		"",
	}
	
	if item.SupplierItemCode != "" {
		elements = append(elements, fmt.Sprintf("%s:SA", item.SupplierItemCode))
	} else {
		elements = append(elements, "")
	}
	
	return EDISegment{Tag: "LIN", Elements: elements}
}

func (g *EDIFACTOrderGenerator) createIMD(item OrderLineItem) EDISegment {
	return EDISegment{
		Tag: "IMD",
		Elements: []string{
			"F",
			"",
			"",
			fmt.Sprintf(":::%s", item.Description),
		},
	}
}

func (g *EDIFACTOrderGenerator) createQTY(item OrderLineItem) EDISegment {
	uom := item.UnitOfMeasure
	if uom == "" {
		uom = "PCE"
	}
	
	return EDISegment{
		Tag: "QTY",
		Elements: []string{
			fmt.Sprintf("21:%v:%s", item.Quantity, uom),
		},
	}
}

func (g *EDIFACTOrderGenerator) createPRI(item OrderLineItem) EDISegment {
	return EDISegment{
		Tag: "PRI",
		Elements: []string{
			fmt.Sprintf("AAA:%v", item.UnitPrice),
		},
	}
}

func (g *EDIFACTOrderGenerator) createMOA(item OrderLineItem) EDISegment {
	return EDISegment{
		Tag: "MOA",
		Elements: []string{
			fmt.Sprintf("203:%v", item.Amount),
		},
	}
}

func (g *EDIFACTOrderGenerator) createCNT(order EDIOrder) EDISegment {
	return EDISegment{
		Tag: "CNT",
		Elements: []string{
			fmt.Sprintf("2:%v", order.TotalLines),
		},
	}
}

func (g *EDIFACTOrderGenerator) createMOATotal(order EDIOrder) EDISegment {
	return EDISegment{
		Tag: "MOA",
		Elements: []string{
			fmt.Sprintf("128:%v", order.TotalAmount),
		},
	}
}

func (g *EDIFACTOrderGenerator) createUNT(order EDIOrder, segments []EDISegment) EDISegment {
	segmentCount := 0
	for _, seg := range segments {
		if seg.Tag == "UNH" {
			segmentCount = 1
		} else if seg.Tag == "UNT" {
			break
		} else {
			segmentCount++
		}
	}
	
	return EDISegment{
		Tag: "UNT",
		Elements: []string{
			fmt.Sprintf("%d", segmentCount),
			order.MessageRefNumber,
		},
	}
}

func (g *EDIFACTOrderGenerator) createUNZ(order EDIOrder, segments []EDISegment) EDISegment {
	messageCount := 1
	for _, seg := range segments {
		if seg.Tag == "UNH" {
			messageCount++
		}
	}
	
	return EDISegment{
		Tag: "UNZ",
		Elements: []string{
			fmt.Sprintf("%d", messageCount),
			order.InterchangeControlRef,
		},
	}
}

type EDIWriter struct {
	OutputDir string
}

func NewEDIWriter(outputDir string) *EDIWriter {
	return &EDIWriter{OutputDir: outputDir}
}

func (w *EDIWriter) WriteOrder(order EDIOrder, content string) (string, error) {
	err := os.MkdirAll(w.OutputDir, 0755)
	if err != nil {
		return "", err
	}
	
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(w.OutputDir, fmt.Sprintf("ORDER_%s_%s.edi", order.OrderNumber, timestamp))
	
	err = os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		return "", err
	}
	
	return filename, nil
}

func (w *EDIWriter) WriteOrderWithPrefix(order EDIOrder, content string, prefix string) (string, error) {
	err := os.MkdirAll(w.OutputDir, 0755)
	if err != nil {
		return "", err
	}
	
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(w.OutputDir, fmt.Sprintf("%s_%s_%s.edi", prefix, order.OrderNumber, timestamp))
	
	err = os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		return "", err
	}
	
	return filename, nil
}

func (w *EDIWriter) WriteOrderToPath(order EDIOrder, content string, filename string) (string, error) {
	err := os.MkdirAll(w.OutputDir, 0755)
	if err != nil {
		return "", err
	}
	
	fullPath := filepath.Join(w.OutputDir, filename)
	
	err = os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		return "", err
	}
	
	return fullPath, nil
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
		
		Items: []OrderLineItem{
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
	
	ediMessage := generator.Generate(order)
	
	filename, err := writer.WriteOrder(order, ediMessage)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("EDIFACT order generated successfully: %s\n", filename)
	fmt.Println("\nEDIFACT Message Content:")
	fmt.Println(ediMessage)
}
