# Gestor de Secretos y Contraseñas TUI PQC (Post-Quantum Cryptography)

Un gestor de secretos ultraligero y seguro basado en TUI, diseñado para operar 100% sobre repositorios Git como almacenamiento y backend de identidad, sin necesidad de servidores centralizados complejos.

> **Estado:** en fase inicial. Aún no hay código de implementación.

## Características clave

- **Espacios de trabajo:** Personal y Empresarial.
- **Jerarquía por ambientes** (`DEV`, `QA`, `PRO`) y categorías.
- **Criptografía resistente a computación cuántica** (estándares NIST):
  - **ML-KEM** (FIPS 203) para encapsulamiento de llaves; la llave compartida resultante cifra los secretos con un AEAD simétrico.
  - **ML-DSA** (FIPS 204) para firmas digitales.
- **Control de acceso** basado en llaves públicas y cifrado por destinatario.
- **Gestión segura de memoria:** borrado directo de búferes en RAM y portapapeles con autolimpieza.

## Tech Stack

| Área          | Tecnología                                                  |
| ------------- | ----------------------------------------------------------- |
| Lenguaje      | Go (Golang)                                                 |
| TUI Framework | `charmbracelet/bubbletea` & `charmbracelet/lipgloss`        |
| Criptografía  | NIST PQC (ML-KEM / ML-DSA vía `cloudflare/circl`)           |
| Storage       | Vaults basados en archivos sobre Git (integración GitHub)   |

## Estructura del proyecto

```
.
├── cmd/        # Puntos de entrada de los binarios
├── internal/   # Código privado de la aplicación
└── pkg/        # Paquetes reutilizables públicos
```

## Licencia

Por definir.
