# github actions workflows

## workflows

| workflow | trigger | target |
|----------|---------|--------|
| `deploy-connect-dev.yml` | push to `main` | https://connect.dev.llmvault.dev |
| `deploy-connect-prod.yml` | release published | https://connect.llmvault.dev |

## setup

### 1. configure aws oidc (recommended)

these workflows use [aws oidc](https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services) instead of long-lived credentials.

#### for development account

```bash
# create oidc provider for github (one per aws account)
aws iam create-open-id-connect-provider \
  --url https://token.actions.githubusercontent.com \
  --thumbprint-list 6938fd4e98bab03faadb97b34396831e3780aea1 \
  --client-id-list sts.amazonaws.com \
  --region us-east-2

# create iam role for github actions
cat > trust-policy.json << 'eof'
{
  "version": "2012-10-17",
  "statement": [
    {
      "effect": "allow",
      "principal": {
        "federated": "arn:aws:iam::your_dev_account_id:oidc-provider/token.actions.githubusercontent.com"
      },
      "action": "sts:assumerolewithwebidentity",
      "condition": {
        "stringequals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "stringlike": {
          "token.actions.githubusercontent.com:sub": "repo:your_org/llmvault.dev:ref:refs/heads/main"
        }
      }
    }
  ]
}
eof

aws iam create-role \
  --role-name githubactionsconnectdeploydev \
  --assume-role-policy-document file://trust-policy.json \
  --region us-east-2

# attach required permissions
aws iam attach-role-policy \
  --role-name githubactionsconnectdeploydev \
  --policy-arn arn:aws:iam::aws:policy/amazons3fullaccess

aws iam attach-role-policy \
  --role-name githubactionsconnectdeploydev \
  --policy-arn arn:aws:iam::aws:policy/cloudfrontfullaccess

# get the role arn
aws iam get-role --role-name githubactionsconnectdeploydev --query 'role.arn' --output text
```

#### for production account

repeat the above with:
- role name: `githubactionsconnectdeployprod`
- condition for releases (optional):
```json
"condition": {
  "stringequals": {
    "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
  },
  "stringlike": {
    "token.actions.githubusercontent.com:sub": "repo:your_org/llmvault.dev:environment:production"
  }
}
```

### 2. add github secrets

go to **settings → secrets and variables → actions** and add:

| secret | value | environment |
|--------|-------|-------------|
| `aws_role_arn_dev` | `arn:aws:iam::dev_account:role/githubactionsconnectdeploydev` | *(none)* |
| `aws_role_arn_prod` | `arn:aws:iam::prod_account:role/githubactionsconnectdeployprod` | *(none)* |

### alternative: use iam user (simpler, less secure)

if you prefer not to use oidc, update the workflows to use access keys:

```yaml
- name: configure aws credentials
  uses: aws-actions/configure-aws-credentials@v4
  with:
    aws-access-key-id: ${{ secrets.aws_access_key_id }}
    aws-secret-access-key: ${{ secrets.aws_secret_access_key }}
    aws-region: us-east-2
```

and add `aws_access_key_id` and `aws_secret_access_key` as secrets.

## testing

### test development deployment

push to main:
```bash
git checkout main
git pull origin main
# make a change to apps/connect/
git add .
git commit -m "test dev deployment"
git push origin main
```

check the **actions** tab in github.

### test production deployment

create a release:
1. go to **releases** in github
2. click **draft a new release**
3. choose a tag (e.g., `v1.0.0`)
4. click **publish release**

this triggers the production deployment.

## troubleshooting

### "could not load credentials"

- check that `aws_role_arn_dev` or `aws_role_arn_prod` is set correctly
- verify the iam role trust policy allows github actions
- ensure `id-token: write` permission is set in the workflow

### "stack not found"

the infrastructure stack must be deployed first:
```bash
cd infrastructure/cdk
npx cdk deploy llmvault-dev-connect  # or llmvault-prod-connect
```

### "access denied" to s3 or cloudfront

the iam role needs these permissions:
- `s3:putobject`, `s3:deleteobject` on the connect assets bucket
- `cloudfront:createinvalidation` on the distribution
