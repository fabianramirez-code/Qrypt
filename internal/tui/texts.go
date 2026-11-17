package tui

import "fmt"

type texts struct {
	spacesTitle, spacesHint, spaceAddTitle, spaceRenameTitle, spaceDeleteTitle, spaceNameHint, spaceExists, spaceNotEmpty string
	previewBanner                                                                                                         string
	welcomeTitle, welcomeBody, welcomeHint                                                                                string
	generating, passTitle, passIntro, passOutro, secretHint                                                               string
	confirmPassTitle, confirmPassPrompt, checkHint, checking                                                              string
	recoveryTitle, recoveryIntro, recoveryOutro                                                                           string
	confirmRecoveryTitle, savingTitle, savingBody                                                                         string
	doneTitle, doneBody, fileLabel, doneOutro, exitHint                                                                   string
	failedTitle, failedBody, failedOutro, discard                                                                         string
	enlarge, unlockTitle, unlockPrompt, unlockHint                                                                        string
	cannotOpenTitle                                                                                                       string
	listTitle, noItems, listHint, listEmptyHint, itemTitle, itemHint, tooLong                                             string
	addTitle, namePrompt, addNameHint, valuePrompt, valueHint, editTitle                                                  string
	deleteTitle, deleteQuestion, deleteHint, vaultFailedTitle, vaultLabel                                                 string
	invalidName, nameExists, valueTooLarge, lineBreak, vaultDamaged, copied                                               string
	unlockWrongPass, invalidIdentity, newerIdentity                                                                       string
	wordCount, unknownWord, wrongPass, pasted, alreadyExists                                                              string
	wordProgress, wordPrompt, wrongWord                                                                                   func(int) string
	typedWords                                                                                                            func(int) string
	welcomeRestore, unlockRestoreHint, restoreTitle, restoreIntro, restoreBackup, restoreIntroHint                        string
	restoreWordTitle, restoreWordHint, rebuilding, newPassTitle                                                           string
	restoredTitle, restoredBody, backupLabel, previousPass, keepNewPass                                                   string
	notRestoredTitle, notRestoredBody, notRestoredOutro, discardNew, phraseStillValid                                     string
	invalidWord, invalidPhrase, backupExists                                                                              string
	restoreProgress, restorePrompt                                                                                        func(int) string
}

var english = texts{
	spacesTitle: "Spaces", spacesHint: "Up/Down: move  Enter: open  A: add  R: rename\nD: delete  Esc: lock", spaceAddTitle: "Add a space", spaceRenameTitle: "Rename space", spaceDeleteTitle: "Delete this space?", spaceNameHint: "Enter: save  Esc: back", spaceExists: "A space with that name already exists.", spaceNotEmpty: "This space is not empty. Delete its items first.",
	previewBanner: "PREVIEW: temporary data, nothing is saved",
	welcomeTitle:  "Creating an identity", welcomeBody: "Qrypt will give you two secrets.\n\nPassphrase (7 words)\n  You type it every time you open Qrypt on this device.\n\nRecovery phrase (24 words)\n  It rebuilds your identity if you lose this device\n  or forget the passphrase.\n\nWrite both on paper. They are shown only now,\nand nobody can reset them.", welcomeHint: "Enter: create   R: restore   Esc: quit", welcomeRestore: "Have a recovery phrase already? Press R to restore.",
	generating: "Generating and sealing your identity. Please wait.", passTitle: "Passphrase", passIntro: "Write these 7 words on paper, in order.", passOutro: "You will type them on the next screen.", secretHint: "Enter: continue  Ctrl+C: quit",
	confirmPassTitle: "Confirm passphrase", confirmPassPrompt: "Type the 7 words in order (visible):", checkHint: "Enter: check  Esc: back", checking: "Checking your passphrase. Please wait.",
	recoveryTitle: "Recovery phrase", recoveryIntro: "Write these 24 words on paper, in order.", recoveryOutro: "Keep the paper somewhere safe, away from this device.\nDo not store it in a photo, a note or a file.",
	confirmRecoveryTitle: "Confirm recovery phrase", savingTitle: "Saving", savingBody: "Saving your identity. Please wait.",
	doneTitle: "Done", doneBody: "Your identity is ready.", fileLabel: "Identity file: ", doneOutro: "Keep both papers safe.", exitHint: "Any key: exit",
	failedTitle: "Not saved", failedBody: "Qrypt could not save your identity:", failedOutro: "Nothing was saved.\nThe words you wrote down belong to no identity.", discard: "Discard them.",
	enlarge:     "Enlarge the window to at least 60 columns by 20 rows.",
	unlockTitle: "Unlock Qrypt", unlockPrompt: "Type your passphrase (7 words):", unlockHint: "Enter: unlock  Tab: show/hide  Esc: quit", unlockRestoreHint: "Forgot the passphrase? Ctrl+R: restore",
	cannotOpenTitle: "Cannot open the identity",
	listTitle:       "Qrypt vault", noItems: "Nothing stored yet. Press A to add an item.", listHint: "Up/Down: move  Enter: open  A: add  D: delete  C: copy\nEsc: spaces", listEmptyHint: "A: add  Esc: spaces", itemTitle: "Item", itemHint: "Tab: show/hide  E: edit  C: copy  Esc: back", tooLong: "The value is too long to show here.",
	addTitle: "Add an item", namePrompt: "Name:", addNameHint: "Enter: next  Esc: back", valuePrompt: "Value (hidden):", valueHint: "Enter: save  Tab: show/hide  Esc: back", editTitle: "Edit value",
	deleteTitle: "Delete this item?", deleteQuestion: "This cannot be undone.", deleteHint: "Y: delete  Any other key: cancel", vaultFailedTitle: "Cannot use the vault", vaultLabel: "Vault: ",
	invalidName: "That name is not valid. Use 1 to 255 characters, with no control characters.", nameExists: "An item with that name already exists.", valueTooLarge: "That value is too large. The limit is 64 KiB.", lineBreak: "Values are one line. Remove the line break.", vaultDamaged: "A vault file is damaged, or the vault belongs to another identity.", copied: "Copied. The clipboard is cleared in 30 seconds.",
	unlockWrongPass: "Wrong passphrase, or the identity file is damaged.", invalidIdentity: "The identity file is not a valid Qrypt identity.", newerIdentity: "This identity was created by a newer version of Qrypt.",
	typedWords: func(n int) string { return fmt.Sprintf("Words typed: %d of 7", n) },
	wordCount:  "Type exactly 7 words.", unknownWord: "One of the words is not valid. Check the spelling.", wrongPass: "That is not the passphrase shown. Check your paper.", pasted: "Pasting is not accepted. Type the words from your paper.", alreadyExists: "An identity file already exists.",
	wordProgress: func(i int) string { return fmt.Sprintf("Word %d of 3", i) }, wordPrompt: func(i int) string { return fmt.Sprintf("Type word %d:", i) }, wrongWord: func(i int) string { return fmt.Sprintf("That is not word %d. Check your paper.", i) },
	restoreTitle: "Restore an identity", restoreIntro: "You need your recovery phrase (24 words).\n\nQrypt will rebuild your identity and give you\na new passphrase. The old passphrase will stop working.", restoreBackup: "The current identity file will be kept as a backup.", restoreIntroHint: "Enter: continue   Esc: back",
	restoreWordTitle: "Recovery phrase", restoreWordHint: "Enter: next  Tab: show/hide  Esc: back", restoreProgress: func(i int) string { return fmt.Sprintf("Word %d of 24", i) }, restorePrompt: func(i int) string { return fmt.Sprintf("Type word %d:", i) }, rebuilding: "Rebuilding your identity. Please wait.", newPassTitle: "New passphrase",
	restoredTitle: "Restored", restoredBody: "Your identity is restored.", backupLabel: "Previous file kept as: ", previousPass: "Your previous passphrase no longer opens Qrypt here.", keepNewPass: "Keep your new passphrase paper safe.",
	notRestoredTitle: "Not restored", notRestoredBody: "Qrypt could not save your identity:", notRestoredOutro: "Nothing was changed.", discardNew: "The new passphrase is not valid. Discard it.", phraseStillValid: "Your recovery phrase is still valid.",
	invalidWord: "That is not a valid word. Check the spelling.", invalidPhrase: "Not a valid recovery phrase. Type the 24 words again.", backupExists: "A backup with the same time already exists. Try again.",
}

var spanish = texts{
	spacesTitle: "Espacios", spacesHint: "↑/↓ mover  Enter abrir  A añadir  R renombrar\nD borrar  Esc bloquear", spaceAddTitle: "Añadir un espacio", spaceRenameTitle: "Renombrar espacio", spaceDeleteTitle: "¿Borrar este espacio?", spaceNameHint: "Enter: guardar  Esc: volver", spaceExists: "Ya existe un espacio con ese nombre.", spaceNotEmpty: "Este espacio no está vacío. Borra primero sus elementos.",
	previewBanner: "VISTA PREVIA: datos temporales, no se guarda nada",
	welcomeTitle:  "Crear una identidad", welcomeBody: "Qrypt te dará dos secretos.\n\nFrase de contraseña (7 palabras)\n  La escribes cada vez que abres Qrypt en este equipo.\n\nFrase de recuperación (24 palabras)\n  Reconstruye tu identidad si pierdes este equipo\n  u olvidas la frase de contraseña.\n\nAnota las dos en papel. Solo se muestran ahora,\ny nadie puede restablecerlas.\n\nLas palabras están en inglés. Cópialas tal cual.", welcomeHint: "Enter: crear   R: restaurar   Esc: salir", welcomeRestore: "¿Ya tienes frase de recuperación? Pulsa R para restaurar.",
	generating: "Preparando tu identidad. Espera un momento.", passTitle: "Frase de contraseña", passIntro: "Anota estas 7 palabras en papel, en orden.", passOutro: "Las escribirás en la siguiente pantalla.", secretHint: "Enter: continuar  Ctrl+C: salir",
	confirmPassTitle: "Confirmar frase de contraseña", confirmPassPrompt: "Escribe las 7 palabras en orden (visibles):", checkHint: "Enter: comprobar  Esc: volver", checking: "Comprobando tu frase de contraseña. Espera un momento.",
	recoveryTitle: "Frase de recuperación", recoveryIntro: "Anota estas 24 palabras en papel, en orden.", recoveryOutro: "Guarda el papel en un lugar seguro, lejos de este equipo.\nNo la guardes en una foto, una nota ni un archivo.",
	confirmRecoveryTitle: "Confirmar frase de recuperación", savingTitle: "Guardando", savingBody: "Guardando tu identidad. Espera un momento.",
	doneTitle: "Listo", doneBody: "Tu identidad está lista.", fileLabel: "Archivo de identidad: ", doneOutro: "Guarda bien los dos papeles.", exitHint: "Cualquier tecla: salir",
	failedTitle: "No se guardó", failedBody: "Qrypt no pudo guardar tu identidad:", failedOutro: "No se guardó nada.\nLas palabras que anotaste no pertenecen a ninguna identidad.", discard: "Deséchalas.",
	enlarge:     "Agranda la ventana a 60 columnas por 20 filas como mínimo.",
	unlockTitle: "Abrir Qrypt", unlockPrompt: "Escribe tu frase de contraseña (7 palabras):", unlockHint: "Enter: abrir  Tab: mostrar/ocultar  Esc: salir", unlockRestoreHint: "¿Olvidaste la frase? Ctrl+R: restaurar",
	cannotOpenTitle: "No se puede abrir la identidad",
	listTitle:       "Bóveda de Qrypt", noItems: "Aún no hay nada guardado. Pulsa A para añadir algo.", listHint: "↑/↓ mover  Enter abrir  A añadir  D borrar  C copiar\nEsc: espacios", listEmptyHint: "A: añadir  Esc: espacios", itemTitle: "Elemento", itemHint: "Tab: mostrar/ocultar  E: editar  C: copiar  Esc: volver", tooLong: "El valor es demasiado largo para mostrarlo aquí.",
	addTitle: "Añadir un elemento", namePrompt: "Nombre:", addNameHint: "Enter: siguiente  Esc: volver", valuePrompt: "Valor (oculto):", valueHint: "Enter: guardar  Tab: mostrar/ocultar  Esc: volver", editTitle: "Editar valor",
	deleteTitle: "¿Borrar este elemento?", deleteQuestion: "No se puede deshacer.", deleteHint: "Y: borrar  Cualquier otra tecla: cancelar", vaultFailedTitle: "No se puede usar la bóveda", vaultLabel: "Bóveda: ",
	invalidName: "Ese nombre no es válido. Usa de 1 a 255 caracteres, sin caracteres de control.", nameExists: "Ya existe un elemento con ese nombre.", valueTooLarge: "Ese valor es demasiado grande. El límite es 64 KiB.", lineBreak: "Los valores son de una línea. Quita el salto de línea.", vaultDamaged: "Un archivo de la bóveda está dañado, o la bóveda es de otra identidad.", copied: "Copiado. El portapapeles se borra en 30 segundos.",
	unlockWrongPass: "Frase incorrecta, o el archivo de identidad está dañado.", invalidIdentity: "El archivo de identidad no es una identidad válida de Qrypt.", newerIdentity: "Esta identidad es de una versión más nueva de Qrypt.",
	typedWords: func(n int) string { return fmt.Sprintf("Palabras escritas: %d de 7", n) },
	wordCount:  "Escribe exactamente 7 palabras.", unknownWord: "Una de las palabras no es válida. Revisa cómo está escrita.", wrongPass: "Esa no es la frase de contraseña mostrada. Revisa tu papel.", pasted: "No se acepta pegar. Escribe las palabras de tu papel.", alreadyExists: "Ya existe un archivo de identidad.",
	wordProgress: func(i int) string { return fmt.Sprintf("Palabra %d de 3", i) }, wordPrompt: func(i int) string { return fmt.Sprintf("Escribe la palabra %d:", i) }, wrongWord: func(i int) string { return fmt.Sprintf("Esa no es la palabra %d. Revisa tu papel.", i) },
	restoreTitle: "Restaurar una identidad", restoreIntro: "Necesitas tu frase de recuperación (24 palabras).\n\nQrypt reconstruirá tu identidad y te dará una nueva\nfrase de contraseña. La anterior dejará de funcionar.", restoreBackup: "El archivo de identidad actual se conservará como respaldo.", restoreIntroHint: "Enter: continuar   Esc: volver",
	restoreWordTitle: "Frase de recuperación", restoreWordHint: "Enter: siguiente  Tab: mostrar/ocultar  Esc: volver", restoreProgress: func(i int) string { return fmt.Sprintf("Palabra %d de 24", i) }, restorePrompt: func(i int) string { return fmt.Sprintf("Escribe la palabra %d:", i) }, rebuilding: "Reconstruyendo tu identidad. Espera un momento.", newPassTitle: "Nueva frase de contraseña",
	restoredTitle: "Identidad restaurada", restoredBody: "Tu identidad está restaurada.", backupLabel: "Archivo anterior conservado como: ", previousPass: "Tu frase de contraseña anterior ya no abre Qrypt aquí.", keepNewPass: "Guarda bien el papel de tu nueva frase de contraseña.",
	notRestoredTitle: "No se restauró", notRestoredBody: "Qrypt no pudo guardar tu identidad:", notRestoredOutro: "No se cambió nada.", discardNew: "La nueva frase de contraseña no es válida. Deséchala.", phraseStillValid: "Tu frase de recuperación sigue siendo válida.",
	invalidWord: "Esa no es una palabra válida. Revisa cómo está escrita.", invalidPhrase: "Frase de recuperación no válida. Escríbela de nuevo.", backupExists: "Ya existe un respaldo con la misma hora. Inténtalo de nuevo.",
}
