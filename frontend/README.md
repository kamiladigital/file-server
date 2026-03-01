# Frontend (Vite + React) — File Uploader

Prerequisites
- Node.js (>=16) and `npm` installed
- Backend running at `http://localhost:8080` (see `../backend`)

Quick start (development)

1. Copy `.env.sample` to `.env` and configure if needed:
   ```bash
   cp .env.sample .env
   ```
   Edit `.env` to change `VITE_API_BASE` if your backend runs on a different host/port.

2. Install dependencies
```bash
cd frontend
npm install
```

3. Run dev server (Vite)
```bash
npm run dev
```

Open the URL printed by Vite (usually http://localhost:3000).

Start backend (separate terminal)

```bash
cd ../backend
go run ./cmd/server
```

Notes
- The frontend talks to the backend endpoints at `http://localhost:8080` by default. If you run the backend on another host/port, update `VITE_API_BASE` in `.env` file.
- The uploader enforces a 1GB limit on both frontend and backend.
- The backend sets minimal CORS headers; ensure your S3 bucket CORS allows `PUT` and exposes `ETag` if needed for browsers.
- For production, build the frontend with `npm run build` and serve the `dist/` output.

Commands (summary)

```bash
# frontend
cd frontend
npm install
npm run dev

# backend (separate terminal)
cd ../backend
go run ./cmd/server
```

Environment Variables
- `VITE_API_BASE`: Backend API base URL (default: `http://localhost:8080`)
