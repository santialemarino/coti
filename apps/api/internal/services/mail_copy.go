package services

import "fmt"

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
)

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
