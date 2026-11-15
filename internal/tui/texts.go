package tui

import "fmt"

type texts struct {
	welcomeTitle, welcomeBody, welcomeHint                   string
	generating, passTitle, passIntro, passOutro, secretHint  string
	confirmPassTitle, confirmPassPrompt, checkHint, checking string
	recoveryTitle, recoveryIntro, recoveryOutro              string
	confirmRecoveryTitle, savingTitle, savingBody            string
	doneTitle, doneBody, fileLabel, doneOutro, exitHint      string
	failedTitle, failedBody, failedOutro, discard            string
	existingTitle, existingBody, enlarge                     string
	wordCount, unknownWord, wrongPass, pasted, alreadyExists string
	wordProgress, wordPrompt, wrongWord                      func(int) string
}

var english = texts{
	welcomeTitle: "Creating an identity", welcomeBody: "Qrypt will give you two secrets.\n\nPassphrase (7 words)\n  You type it every time you open Qrypt on this device.\n\nRecovery phrase (24 words)\n  It rebuilds your identity if you lose this device\n  or forget the passphrase.\n\nWrite both on paper. They are shown only now,\nand nobody can reset them.", welcomeHint: "Enter: create   Esc: quit",
	generating: "Generating and sealing your identity. Please wait.", passTitle: "Passphrase", passIntro: "Write these 7 words on paper, in order.", passOutro: "You will type them on the next screen.", secretHint: "Enter: continue  Ctrl+C: quit",
	confirmPassTitle: "Confirm passphrase", confirmPassPrompt: "Type the 7 words in order (visible):", checkHint: "Enter: check  Esc: back", checking: "Checking your passphrase. Please wait.",
	recoveryTitle: "Recovery phrase", recoveryIntro: "Write these 24 words on paper, in order.", recoveryOutro: "Keep the paper somewhere safe, away from this device.\nDo not store it in a photo, a note or a file.",
	confirmRecoveryTitle: "Confirm recovery phrase", savingTitle: "Saving", savingBody: "Saving your identity. Please wait.",
	doneTitle: "Done", doneBody: "Your identity is ready.", fileLabel: "Identity file: ", doneOutro: "Keep both papers safe.", exitHint: "Any key: exit",
	failedTitle: "Not saved", failedBody: "Qrypt could not save your identity:", failedOutro: "Nothing was saved.\nThe words you wrote down belong to no identity.", discard: "Discard them.",
	existingTitle: "Identity exists", existingBody: "Unlocking is not available yet.", enlarge: "Enlarge the window to at least 60 columns by 20 rows.",
	wordCount: "Type exactly 7 words.", unknownWord: "One of the words is not valid. Check the spelling.", wrongPass: "That is not the passphrase shown. Check your paper.", pasted: "Pasting is not accepted. Type the words from your paper.", alreadyExists: "An identity file already exists.",
	wordProgress: func(i int) string { return fmt.Sprintf("Word %d of 3", i) }, wordPrompt: func(i int) string { return fmt.Sprintf("Type word %d:", i) }, wrongWord: func(i int) string { return fmt.Sprintf("That is not word %d. Check your paper.", i) },
}

var spanish = texts{
	welcomeTitle: "Crear una identidad", welcomeBody: "Qrypt te dará dos secretos.\n\nFrase de contraseña (7 palabras)\n  La escribes cada vez que abres Qrypt en este equipo.\n\nFrase de recuperación (24 palabras)\n  Reconstruye tu identidad si pierdes este equipo\n  u olvidas la frase de contraseña.\n\nAnota las dos en papel. Solo se muestran ahora,\ny nadie puede restablecerlas.\n\nLas palabras están en inglés. Cópialas tal cual.", welcomeHint: "Enter: crear   Esc: salir",
	generating: "Preparando tu identidad. Espera un momento.", passTitle: "Frase de contraseña", passIntro: "Anota estas 7 palabras en papel, en orden.", passOutro: "Las escribirás en la siguiente pantalla.", secretHint: "Enter: continuar  Ctrl+C: salir",
	confirmPassTitle: "Confirmar frase de contraseña", confirmPassPrompt: "Escribe las 7 palabras en orden (visibles):", checkHint: "Enter: comprobar  Esc: volver", checking: "Comprobando tu frase de contraseña. Espera un momento.",
	recoveryTitle: "Frase de recuperación", recoveryIntro: "Anota estas 24 palabras en papel, en orden.", recoveryOutro: "Guarda el papel en un lugar seguro, lejos de este equipo.\nNo la guardes en una foto, una nota ni un archivo.",
	confirmRecoveryTitle: "Confirmar frase de recuperación", savingTitle: "Guardando", savingBody: "Guardando tu identidad. Espera un momento.",
	doneTitle: "Listo", doneBody: "Tu identidad está lista.", fileLabel: "Archivo de identidad: ", doneOutro: "Guarda bien los dos papeles.", exitHint: "Cualquier tecla: salir",
	failedTitle: "No se guardó", failedBody: "Qrypt no pudo guardar tu identidad:", failedOutro: "No se guardó nada.\nLas palabras que anotaste no pertenecen a ninguna identidad.", discard: "Deséchalas.",
	existingTitle: "Ya existe una identidad", existingBody: "Abrirla todavía no está disponible.", enlarge: "Agranda la ventana a 60 columnas por 20 filas como mínimo.",
	wordCount: "Escribe exactamente 7 palabras.", unknownWord: "Una de las palabras no es válida. Revisa cómo está escrita.", wrongPass: "Esa no es la frase de contraseña mostrada. Revisa tu papel.", pasted: "No se acepta pegar. Escribe las palabras de tu papel.", alreadyExists: "Ya existe un archivo de identidad.",
	wordProgress: func(i int) string { return fmt.Sprintf("Palabra %d de 3", i) }, wordPrompt: func(i int) string { return fmt.Sprintf("Escribe la palabra %d:", i) }, wrongWord: func(i int) string { return fmt.Sprintf("Esa no es la palabra %d. Revisa tu papel.", i) },
}
