# Superset configuration for testing the provider. It enables the features that some managed
# resources need. Do not use it in production: the secret key is public.
import os

SECRET_KEY = os.environ["SUPERSET_SECRET_KEY"]

# Superset keeps its own metadata in PostgreSQL, as in production. SQLite would reuse the ID of
# a deleted object for the next one.
SQLALCHEMY_DATABASE_URI = "postgresql+psycopg2://examples:examples@postgres:5432/superset"

# The users, roles, permissions and groups endpoints of Flask-AppBuilder.
FAB_ADD_SECURITY_API = True

FEATURE_FLAGS = {
    "EMBEDDED_SUPERSET": True,  # DashboardEmbedding
    "SSH_TUNNELING": True,  # Database.sshTunnel
    "ENABLE_TEMPLATE_PROCESSING": True,  # Jinja in datasets and saved queries
    "ALERT_REPORTS": True,  # Report
    "TAGGING_SYSTEM": True,  # Tag, tags of charts and dashboards
    "ENABLE_SUPERSET_META_DB": True,  # MetaDatabase
    "DASHBOARD_RBAC": True,  # the roles of a Dashboard
}
