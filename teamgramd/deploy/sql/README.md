# Legacy Schema Archive

The SQL files in this directory are retained only as historical migration
references. They are not a supported deployment input and must not be loaded
by production or test services.

All new environments use PostgreSQL 18 migrations from
`teamgramd/deploy/sql/postgres` and the runner in `teamgramd/deploy/postgres`.
