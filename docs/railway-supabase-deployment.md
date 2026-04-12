# Railway + Supabase Deployment Guide

## Goal
Deploy the Go API on Railway while keeping PostgreSQL on Supabase, using a `DATABASE_URL` that is safe for a long-lived backend service.

## Recommended Strategy
For this app, keep:
- Railway for the API runtime
- Supabase for PostgreSQL

Use one of these database connection modes:

1. Preferred when your Railway runtime and network path support it reliably:
- Supabase direct connection on `:5432`

2. Recommended default when you want the safer general Railway setup:
- Supavisor session mode on `:5432`

Do not use:
- Supabase transaction pool mode on `:6543`

This backend is a persistent Go API, not a short-lived serverless worker. Transaction pool mode is the wrong default because it is less compatible with long-lived server connection behavior.

## Allowed Production DATABASE_URL Shapes
### Supabase direct connection
```env
DATABASE_URL=postgres://postgres:<PASSWORD>@db.<PROJECT-REF>.supabase.co:5432/postgres?sslmode=require
```

### Supabase session pooler
```env
DATABASE_URL=postgres://postgres.<PROJECT-REF>:<PASSWORD>@aws-0-<REGION>.pooler.supabase.com:5432/postgres?sslmode=require
```

## Rejected Production DATABASE_URL Shape
### Supabase transaction pooler
```env
DATABASE_URL=postgres://postgres.<PROJECT-REF>:<PASSWORD>@aws-0-<REGION>.pooler.supabase.com:6543/postgres?sslmode=require
```

The application now rejects this shape in `APP_ENV=production`.

## Railway Setup
- Deploy only the API service to Railway.
- Keep Supabase as the single PostgreSQL system of record.
- Set `DATABASE_URL` in Railway service variables.
- Set `APP_ENV=production`.
- Keep `sslmode=require`.
- Put the Railway service in the region closest to your Supabase project.
- Configure a Railway healthcheck for the API.

## Why This Setup
- It avoids an unnecessary second PostgreSQL cluster on Railway.
- It preserves Supabase as the managed database platform.
- It keeps the app on a connection mode that fits a persistent Go API.
- It keeps production TLS required at the app-config level.

## Residual Tradeoffs
- Railway to Supabase is still cross-service network traffic, so latency can be higher than a colocated Railway database.
- If latency or connection stability becomes a measured problem, reevaluate colocating app and database.
- Distributed rate limiting and edge DDoS protection still depend on the wider deployment architecture, not this database choice alone.
