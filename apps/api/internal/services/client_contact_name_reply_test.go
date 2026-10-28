package services

import "testing"

func TestContactNameReply_AcceptsNamesInTheFormsCustomersWrite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ text, name string }{
		{"Juan Pérez", "Juan Pérez"},
		{"juan perez", "juan perez"},
		{"Soy Juan Pérez", "Juan Pérez"},
		{"soy María José Fernández.", "María José Fernández"},
		{"Me llamo Ana Gómez", "Ana Gómez"},
		{"Mi nombre es Carlos Ruiz Díaz", "Carlos Ruiz Díaz"},
		{"Nombre y apellido: Juan Perez", "Juan Perez"},
		{"Hola, soy Lucía Martínez", "Lucía Martínez"},
		{"Buen día! Roberto D'Alessandro", "Roberto D'Alessandro"},
		{"Hola\nJuan Pérez\nSaludos\nJuan", "Juan Pérez"},
		{"Juan Pérez, gracias!", "Juan Pérez"},
		{"Ana María de la Fuente", "Ana María de la Fuente"},
		{"Pedro Gómez-Ibáñez", "Pedro Gómez-Ibáñez"},
		{"Ñato Muñoz", "Ñato Muñoz"},
	} {
		name, ok := contactNameReply(tc.text)
		if !ok || name != tc.name {
			t.Errorf("%q: got %q, %v; want %q", tc.text, name, ok, tc.name)
		}
	}
}

func TestContactNameReply_LeavesOrdersAndSmallTalkToTheNormalFlow(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"2 bolsas de cemento",
		"dos bolsas de cemento",
		"necesito arena",
		"Cemento Loma Negra",
		"Arena fina",
		"10 ladrillos huecos",
		"¿Tienen hierro del 8?",
		"Cuánto sale el porcelanato",
		"Quiero un presupuesto",
		"Hola",
		"Buen día",
		"Gracias",
		"Muchas gracias",
		"Ok dale",
		"Si",
		"Juan Pérez 1134567890",
		"juan@example.com",
		"Juan Pérez, Belgrano 123",
		"Juan Pérez\nY necesito 3 bolsas de cal",
		"Juan Pérez\nSan Martín",
		"Gracias\nJuan Pérez",
		"Me pasás el precio",
		"Para mañana",
		"Juan Carlos Pedro Luis Alberto Gómez",
		"Abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz abcdefghijklmn",
		"",
	} {
		if name, ok := contactNameReply(text); ok {
			t.Errorf("%q was taken as the name %q", text, name)
		}
	}
}

func TestEmailReplyText_DropsQuotedMessageAndSignature(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, want string }{
		{"Juan Pérez\n\nEl lun, 27 oct 2026 a las 10:00, Corralón Norte <ventas@norte.test>\nescribió:\n> Gracias por escribirnos.\n> Respondé con tu nombre.", "Juan Pérez"},
		{"Juan Pérez\r\n\r\nOn Mon, Oct 27, 2026 at 10:00 AM Corralón <v@n.test> wrote:\r\n> Respondé", "Juan Pérez"},
		{"Juan Pérez\n-- \nJuan Pérez | Constructora JP\nTel 11 5555 5555", "Juan Pérez"},
		{"Soy Juan Pérez\n\nEnviado desde mi iPhone", "Soy Juan Pérez"},
		{"Juan Pérez\nSent from my Galaxy", "Juan Pérez"},
		{"> Respondé con tu nombre\nJuan Pérez", "Juan Pérez"},
		{"Juan Pérez\n\n-----Mensaje original-----\nDe: Corralón\nRespondé", "Juan Pérez"},
		{"Juan Pérez\n\nDe: Corralón Norte <ventas@norte.test>\nEnviado: lunes\nAsunto: Tu pedido", "Juan Pérez"},
		{"Juan Pérez El lun, 27 oct 2026, Corralón <v@n.test> escribió: Gracias por escribirnos", "Juan Pérez"},
	} {
		got := emailReplyText(tc.body)
		if got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.body, got, tc.want)
		}
		if name, ok := contactNameReply(got); !ok || name != "Juan Pérez" {
			t.Errorf("%q: stripped reply %q not recognized (%q)", tc.body, got, name)
		}
	}
}

func TestAutomatedEmailSender_RecognizesAddressesThatNeverRead(t *testing.T) {
	t.Parallel()
	for address, automated := range map[string]bool{
		"no-reply@proveedor.test":      true,
		"noreply@proveedor.test":       true,
		"do_not_reply@banco.test":      true,
		"MAILER-DAEMON@mx.test":        true,
		"postmaster@mx.test":           true,
		"bounces+123@mailer.test":      true,
		"juan.perez@gmail.com":         false,
		"replyto@proveedor.test":       false,
		"compras@constructora-jp.test": false,
		"not-an-address":               true,
	} {
		if got := automatedEmailSender(address); got != automated {
			t.Errorf("%s: got %v", address, got)
		}
	}
}
