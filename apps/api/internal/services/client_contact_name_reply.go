package services

import (
	_ "embed"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The reply heuristic's limits are properties of the rule, not deployment tuning: raising them
// lets short orders through as names, so they are constants rather than settings.
const (
	// nameReplyWindow is WhatsApp's customer-service window, the span decision 79 allows a reply in.
	nameReplyWindow = 24 * time.Hour
	// maxNameReplyLength bounds a recognized reply in characters, after greetings are removed.
	maxNameReplyLength = 60
	// minNameReplyWords asks for first and last name, which is what the prompt requests.
	minNameReplyWords = 2
	// maxNameReplyWords bounds a recognized reply in words; Argentine full names rarely pass four.
	// Particles such as «de la» do not count toward either bound.
	maxNameReplyWords = 5
)

var (
	nameReplyGreeting = regexp.MustCompile(`(?i)^(?:hola|buen(?:[oa]s?)?(?:\s+(?:d[ií]as?|tardes|noches))?|qu[eé]\s+tal)\b[\s,.!]*`)
	nameReplyPrefix   = regexp.MustCompile(`(?i)^(?:(?:soy|me\s+llamo|mi\s+nombre\s+es)\b|nombre\s+y\s+apellido\s*:|nombre\s*:)\s*`)
	nameReplyThanks   = regexp.MustCompile(`(?i)[\s,.!]+(?:muchas\s+)?gracias[\s.!]*$`)

	// emailQuoteStart finds where a client starts quoting the message being answered. It is not
	// anchored to a line because HTML-only mail arrives flattened onto one line.
	emailQuoteStart      = regexp.MustCompile(`(?s)(?:\bEl\s.{1,300}?escribi[oó]:|\bOn\s.{1,300}?wrote:|-{2,}\s*(?:Mensaje original|Original Message|Forwarded message|Mensaje reenviado)\s*-{2,}|\b(?:De|From):\s[^\n]{1,300}\n\s*(?:Enviado|Fecha|Sent|Date):)`)
	emailMobileSignature = regexp.MustCompile(`(?i)^(?:enviado desde mi|sent from my|obtener outlook para|get outlook for|descarg[aá] outlook para)\b`)
	automatedLocalParts  = map[string]bool{"noreply": true, "donotreply": true, "mailerdaemon": true, "postmaster": true}
	// nameReplyAddress catches a link or an address, which only letters and periods would let through.
	nameReplyAddress = regexp.MustCompile(`(?i)@|://|\bwww\.|\.[a-z]{2,}\b`)
)

// Words that make a reply an order rather than a name. A miss only means the reply is processed as
// an order, which is the safe direction.
var nameReplyOrderWords = wordSet(`necesito necesitaria necesitamos quiero queria quisiera mandame mandar manden enviar envien
	enviame traer traigan pedido pedir presupuesto cotizacion cotizar precio precios cuanto cuanta cuantos cuantas sale
	salen cuesta tenes tienen hay stock entrega retiro bolsa bolsas bolson bolsones cemento arena cal ladrillo ladrillos
	ladrillon hierro hierros piedra cascote granza ripio tosca viga vigas vigueta viguetas chapa chapas cano canos tubo
	tubos ceramica ceramicos porcelanato pegamento yeso placa placas durlock malla mallas alambre clavo clavos tornillo
	tornillos pintura membrana hormigon revoque mezcla plasticor kilo kilos kg metro metros m2 m3 unidad unidades docena
	pallet rollo rollos balde baldes litro litros perfil perfiles aislante teja tejas block bloque bloques urgente hoy
	manana hidrofugo
	confirmo confirmado confirmamos confirmar confirma acepto aceptamos rechazo paso pasa busco buscar buscarlo mando
	manda factura facturar facturas envio envia direccion foto fotos listo ya
	el lo le te me se mi tu su al con para por en una ahora siempre esta estoy eso esto aca alla aqui
	klaukol sika sikaflex weber plavicon isopor telgopor holcim minetti acindar rotoplas tersuave sinteplast loma
	ceresita iggam tacuru
	uno una dos tres cuatro cinco seis siete ocho nueve diez once doce veinte media medio doble mitad mas menos otro
	otra agrega agregale agregame suma sumale sumame saca sacale sacame quita quitale cambia cambiale cambiame pone
	ponele mejor`)

// nameReplyOrderPrefixes catch the conjugations and clitic forms of order verbs («cancelalo»).
var nameReplyOrderPrefixes = []string{"cancel", "confirm"}

// Particles join the parts of a surname («de la Torre») and do not count as name words.
var nameReplyParticles = wordSet(`de del la las los y da di van von d`)

// Words that carry no name: a line made only of them is a greeting, an acknowledgement or a closing.
var nameReplyCourtesyWords = wordSet(`hola buenas buen buenos dia dias tardes noches que tal como va ok okey dale si no listo
	perfecto genial barbaro joya bueno chau gracias muchas mil de nada saludos cordiales slds atte atentamente abrazo un`)

// nameReplyClosings end the reply: what follows is a signature.
var nameReplyClosings = wordSet(`gracias saludos slds atte atentamente abrazo cordiales chau`)

// givenNamesList holds common Spanish and Argentine given names, lowercase and unaccented. Compiled
// for Coti from common usage (names are facts); no third-party list is copied.
//
//go:embed given_names.txt
var givenNamesList string

// nameReplyGivenNames anchors the rule: a reply is a name only when its first word is a given name, so
// an unknown short message stays a message rather than becoming the customer's name.
var nameReplyGivenNames = wordSet(givenNamesList)

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

// contactNameReply recognizes a reply to the name prompt: one short line of two to five name words
// that starts with a known given name, letters only, with no digits, link, order or confirmation
// vocabulary, after greetings, «soy» and closings are removed. The account's own catalog is checked
// separately by the caller.
func contactNameReply(text string) (string, bool) {
	var content []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		words := foldedWords(line)
		if len(words) > 0 && allIn(words, nameReplyCourtesyWords) {
			if anyIn(words, nameReplyClosings) {
				break
			}
			continue
		}
		content = append(content, line)
	}
	if len(content) != 1 || strings.ContainsFunc(content[0], unicode.IsDigit) || nameReplyAddress.MatchString(content[0]) {
		return "", false
	}
	name := nameReplyGreeting.ReplaceAllString(content[0], "")
	name = nameReplyPrefix.ReplaceAllString(name, "")
	name = nameReplyThanks.ReplaceAllString(name, "")
	name = strings.Join(strings.Fields(strings.TrimRight(name, " .!")), " ")
	if name == "" || utf8.RuneCountInString(name) > maxNameReplyLength {
		return "", false
	}
	letters := 0
	for _, r := range name {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsMark(r), r == ' ', r == '\'', r == '’', r == '-', r == '.':
		default:
			return "", false
		}
	}
	words := foldedWords(name)
	if letters < 2 || !nameReplyGivenNames[words[0]] || anyIn(words, nameReplyOrderWords) || allIn(words, nameReplyCourtesyWords) {
		return "", false
	}
	nameWords := 0
	for _, word := range words {
		for _, prefix := range nameReplyOrderPrefixes {
			if strings.HasPrefix(word, prefix) {
				return "", false
			}
		}
		if !nameReplyParticles[word] {
			nameWords++
		}
	}
	if nameWords < minNameReplyWords || nameWords > maxNameReplyWords {
		return "", false
	}
	return name, true
}

// emailReplyText keeps what a client wrote above the quoted message and the signature.
func emailReplyText(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if loc := emailQuoteStart.FindStringIndex(body); loc != nil {
		body = body[:loc[0]]
	}
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.TrimRight(line, " \t") == "--" || emailMobileSignature.MatchString(trimmed) {
			break
		}
		// Outlook draws a rule of underscores above the quoted headers.
		if strings.HasPrefix(trimmed, ">") || (trimmed != "" && strings.Trim(trimmed, "_-=") == "") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// automatedEmailSender reports an address no automatic reply may go to.
func automatedEmailSender(address string) bool {
	local, _, found := strings.Cut(strings.ToLower(strings.TrimSpace(address)), "@")
	if !found {
		return true
	}
	local = strings.NewReplacer("-", "", "_", "", ".", "").Replace(local)
	return automatedLocalParts[local] || strings.HasPrefix(local, "bounce")
}

func foldedWords(text string) []string {
	return strings.FieldsFunc(accentFolder.Replace(strings.ToLower(text)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func allIn(words []string, set map[string]bool) bool {
	for _, word := range words {
		if !set[word] {
			return false
		}
	}
	return true
}

func anyIn(words []string, set map[string]bool) bool {
	for _, word := range words {
		if set[word] {
			return true
		}
	}
	return false
}
