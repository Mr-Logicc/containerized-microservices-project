# Injected into auth and orders as the API_KEY environment variable.
resource "aws_secretsmanager_secret" "api_key" {
  name        = "${var.project_name}/api-key1"
  description = "Fake API key / DB password demonstrating Secrets Manager injection. Not a real credential."
}

resource "aws_secretsmanager_secret_version" "api_key" {
  secret_id     = aws_secretsmanager_secret.api_key.id
  secret_string = var.api_key_secret_value
}
