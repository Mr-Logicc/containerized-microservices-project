data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "frontend" {
  bucket = "${var.project_name}-frontend-${data.aws_caller_identity.current.account_id}"
}

# Bucket stays fully private; CloudFront reaches it only via Origin Access
# Control (OAC), not a public bucket policy or ACL.
resource "aws_s3_bucket_public_access_block" "frontend" {
  bucket                  = aws_s3_bucket.frontend.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}



# Scoped to this specific distribution via the SourceArn condition.
resource "aws_s3_bucket_policy" "frontend" {
  bucket = aws_s3_bucket.frontend.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "AllowCloudFrontServicePrincipalReadOnly"
      Effect    = "Allow"
      Principal = { Service = "cloudfront.amazonaws.com" }
      Action    = "s3:GetObject"
      Resource  = "${aws_s3_bucket.frontend.arn}/*"
      Condition = {
        StringEquals = {
          "AWS:SourceArn" = aws_cloudfront_distribution.this.arn
        }
      }
    }]
  })
}

# CI overwrites this object directly via `aws s3 sync`; ignore drift.
resource "aws_s3_object" "frontend_placeholder" {
  bucket       = aws_s3_bucket.frontend.id
  key          = "index.html"
  content      = "<html><body><h1>Deploying...</h1><p>Push to main to deploy the frontend via GitHub Actions.</p></body></html>"
  content_type = "text/html"

  lifecycle {
    ignore_changes = [content, etag, source]
  }
}
