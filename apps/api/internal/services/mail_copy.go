package services

import (
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Product copy, not documentation: an Argentine corralón reads it. This file is the API's
// counterpart to the web apps' es-AR catalog and the only Spanish in the backend.

const (
	mailActionFallback = "Si el botón no funciona, copiá este enlace en tu navegador:"

	passwordResetSubject = "Restablecé tu contraseña"
	passwordResetHeading = "Restablecé tu contraseña"
	passwordResetAction  = "Elegir una contraseña nueva"
	passwordResetIgnore  = "Si no pediste este cambio, ignorá este correo: tu contraseña actual sigue funcionando."

	emailVerificationSubject = "Confirmá tu dirección de correo"
	emailVerificationHeading = "Confirmá tu dirección de correo"
	emailVerificationAction  = "Confirmar mi correo"

	emailChangedSubject = "Cambió la dirección de tu cuenta"
	emailChangedHeading = "Cambió la dirección de tu cuenta"
	// The old mailbox gets no other warning, so it says what the change implies and who to tell.
	emailChangedWarning = "Si no fuiste vos, quien lo hizo tenía tu contraseña: avisale al administrador de tu corralón para que revise el acceso a la cuenta."
	// An administrator made the change, so the warning says who to ask rather than what leaked.
	emailChangedByAdminWarning = "El cambio lo hizo un administrador de tu corralón. Si no lo esperabas, consultalo con él."

	inviteSubject = "Te invitaron a Coti"
	inviteHeading = "Te invitaron a Coti"
	inviteAction  = "Elegir mi contraseña"
	inviteIgnore  = "Si no esperabas esta invitación, ignorá este correo: nadie puede entrar con tu dirección sin este enlace."

	// Decision 79: the same fixed text goes to every unknown sender, so it reveals no profile data.
	contactNamePrompt = "Gracias por escribirnos. Para registrar tus pedidos a tu nombre, respondé solo con tu nombre y apellido."

	contactVerificationSubject     = "Confirmá tu contacto"
	contactRegistrationLinkSubject = "Completá tus datos de facturación"
)

// contactNamePromptSubject names the supplier, never the platform, because the prompt answers mail
// the customer sent to the supplier's own mailbox.
func contactNamePromptSubject(supplier string) string {
	return "Tu pedido a " + supplier
}

// contactVerificationLink invites the customer to open the one-use link that proves a contact.
func contactVerificationLink(link string) string {
	return "Confirmá tu contacto para asociarlo a tu cuenta: " + link
}

// contactVerificationCode carries the one-use code that proves a phone.
func contactVerificationCode(code string) string {
	return "Tu código para confirmar el teléfono en Coti es: " + code
}

// contactRegistrationLink sends a customer to the account where billing data is completed.
func contactRegistrationLink(link string) string {
	return "Para completar tus datos de facturación, ingresá a tu cuenta o creala en " + link + ". Si ya tenés una cuenta con tu CUIT/CUIL, usá esa misma cuenta y agregá el contacto con el que hiciste el pedido."
}

// passwordResetIntro greets the user by name and states what the link is for.
func passwordResetIntro(name string) string {
	return fmt.Sprintf("Hola %s, recibimos un pedido para restablecer la contraseña de tu cuenta.", name)
}

// passwordResetValidity states how long the link lasts, in whole hours or minutes.
func passwordResetValidity(minutes int) string {
	if minutes%60 == 0 && minutes >= 60 {
		hours := minutes / 60
		if hours == 1 {
			return "El enlace vence en 1 hora y se puede usar una sola vez."
		}
		return fmt.Sprintf("El enlace vence en %d horas y se puede usar una sola vez.", hours)
	}
	return fmt.Sprintf("El enlace vence en %d minutos y se puede usar una sola vez.", minutes)
}

// emailVerificationIntro greets the user and says what confirming is for.
func emailVerificationIntro(name string) string {
	return fmt.Sprintf("Hola %s, gracias por registrar tu corralón en Coti. Confirmá tu dirección para que podamos usarla con seguridad.", name)
}

// inviteIntro greets the invited user and says what the link is for.
func inviteIntro(name string) string {
	return fmt.Sprintf("Hola %s, te crearon una cuenta en Coti para trabajar con tu corralón. Elegí tu contraseña para entrar.", name)
}

// inviteValidity states how long the invite lasts, in whole days or hours, and that it works once.
func inviteValidity(hours int) string {
	const tail = " y se puede usar una sola vez. Si se vence, pedile a un administrador que te la reenvíe."
	switch {
	case hours == 24:
		return "La invitación vence en 1 día" + tail
	case hours%24 == 0 && hours > 24:
		return fmt.Sprintf("La invitación vence en %d días", hours/24) + tail
	case hours == 1:
		return "La invitación vence en 1 hora" + tail
	default:
		return fmt.Sprintf("La invitación vence en %d horas", hours) + tail
	}
}

// emailVerificationNewAddressIntro greets the user at the address their account just moved to.
func emailVerificationNewAddressIntro(name string) string {
	return fmt.Sprintf("Hola %s, la dirección de tu cuenta de Coti pasó a ser esta. Confirmala para que podamos usarla con seguridad.", name)
}

// emailChangedIntro tells the old address which address replaced it.
func emailChangedIntro(name, newEmail string) string {
	return fmt.Sprintf("Hola %s, la dirección de tu cuenta de Coti pasó a ser %s. Esta casilla ya no recibe los avisos de la cuenta.", name, newEmail)
}

// emailVerificationValidity states how long the link lasts, in whole hours or days.
func emailVerificationValidity(hours int) string {
	if hours%24 == 0 && hours >= 24 {
		days := hours / 24
		if days == 1 {
			return "El enlace vence en 1 día y se puede usar una sola vez."
		}
		return fmt.Sprintf("El enlace vence en %d días y se puede usar una sola vez.", days)
	}
	return fmt.Sprintf("El enlace vence en %d horas y se puede usar una sola vez.", hours)
}

// quoteOutcomeCopy words what a customer decided, for the seller who sent the quote. The reference
// carries the subject because a seller with several quotes out reads the list, not the mail.
func quoteOutcomeCopy(outcome domain.ClientQuoteOutcome) (subject, heading, body string) {
	if outcome.Action == domain.ClientActionAccept {
		return fmt.Sprintf("%s: el cliente aceptó la cotización", outcome.Reference),
			"Te aceptaron la cotización",
			fmt.Sprintf("El cliente aceptó %s. Entrá al backoffice para coordinar la entrega.",
				outcome.Reference)
	}
	return fmt.Sprintf("%s: el cliente rechazó la cotización", outcome.Reference),
		"Te rechazaron la cotización",
		fmt.Sprintf("El cliente rechazó %s. Si querés retomarla, reactivala desde el backoffice.",
			outcome.Reference)
}
