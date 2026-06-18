# AutoClocking Backend

Backend en Go para AutoClocking. Expone la API de marcajes y la administracion de RUTs, y soporta distintos backends de almacenamiento segun variables de entorno.

## Proposito

Su trabajo es servir como capa de negocio y persistencia para:

- Registrar y listar marcajes.
- Administrar RUTs activos e inactivos.
- Sembrar los RUTs iniciales desde `ACTIVE_RUTS` o `ACTIVE_RUTS_B64`.
- Mantener compatibilidad con almacenamiento local, Firestore o Cloud Storage.

## Arquitectura

```text
Go HTTP server
  |
  +--> /marcajes
  |      |
  |      +--> marcajes store
  |             +--> file
  |             +--> Firestore
  |
  +--> /ruts
         |
         +--> ruts store
                +--> file
                +--> Firestore
                +--> GCS
```

El arranque vive en `cmd/marcajes-api/main.go`. Alli se elige el backend de persistencia segun variables de entorno, se cargan los RUTs iniciales y se levanta el servidor HTTP.

## Funcionamiento

- `GET /marcajes` lista marcajes.
- `POST /marcajes` crea un marcaje.
- `GET /ruts` lista los RUTs configurados.
- `POST /ruts` crea un RUT.
- `PATCH /ruts/{rut}` cambia su estado activo/inactivo.
- `DELETE /ruts/{rut}` elimina un RUT.

La configuracion de ejecucion se lee desde `.env` o variables del entorno. Los RUTs iniciales pueden venir en JSON plano o en base64.

## Variables de entorno

Variables principales:

- `FIRESTORE_PROJECT_ID`
- `GOOGLE_CLOUD_PROJECT`
- `GCP_PROJECT`
- `MARCAJES_STORAGE_BACKEND`
- `MARCAJE_STORAGE_FILE`
- `RUT_STORAGE_BACKEND`
- `RUT_STORAGE_FILE`
- `RUT_STORAGE_BUCKET`
- `RUT_STORAGE_OBJECT`
- `ACTIVE_RUTS`
- `ACTIVE_RUTS_B64`
- `CLOCK_IN_ACTIVE`
- `DEBUG_MODE`

## Comandos

```bash
go run ./cmd/marcajes-api
go test ./...
```

## Maintainer

Camilo Gonzalez <camilo@camiloengineer.com>
