# Deployment Guide for ui.lux.finance

This guide explains how to deploy the Lux Consensus Documentation site to ui.lux.finance.

## 🚀 Quick Deploy

### Option 1: Vercel (Recommended)

1. **Deploy with Vercel CLI:**
```bash
cd /Users/z/work/lux/consensus
vercel --prod
```

2. **Add Custom Domain:**
   - Go to Vercel Dashboard
   - Select your project
   - Go to Settings → Domains
   - Add `ui.lux.finance`
   - Configure DNS:
     - Add CNAME record: `ui` → `cname.vercel-dns.com`
     - Or A record to Vercel's IP

### Option 2: GitHub Pages

1. **Push to GitHub:**
```bash
git add .
git commit -m "Deploy Lux Consensus Documentation"
git push origin main
```

2. **Enable GitHub Pages:**
   - Go to Repository Settings
   - Navigate to Pages section
   - Source: Deploy from a branch
   - Branch: main
   - Folder: /docs
   - Custom domain: ui.lux.finance

3. **DNS Configuration:**
   - Add CNAME record: `ui` → `luxfi.github.io`
   - Or A records to GitHub Pages IPs:
     - 185.199.108.153
     - 185.199.109.153
     - 185.199.110.153
     - 185.199.111.153

### Option 3: Netlify

1. **Connect Repository:**
   - Go to Netlify Dashboard
   - Import from Git
   - Select the repository

2. **Build Settings:**
   - Build command: `cd docs-site && npm run build`
   - Publish directory: `docs`

3. **Custom Domain:**
   - Add domain: ui.lux.finance
   - Configure DNS as directed

## 📦 Build Locally

```bash
# Navigate to project
cd /Users/z/work/lux/consensus/docs-site

# Install dependencies
npm install

# Build for production
npm run build

# Files will be in ../docs/
```

## 🔧 Configuration Files

- **vercel.json**: Vercel deployment configuration
- **.github/workflows/deploy.yml**: GitHub Actions deployment
- **docs/CNAME**: GitHub Pages custom domain
- **public/CNAME**: Source CNAME file (copied during build)

## 🌐 DNS Configuration

For ui.lux.finance to work, configure DNS with your domain provider:

### For Vercel:
```
Type: CNAME
Name: ui
Value: cname.vercel-dns.com
```

### For GitHub Pages:
```
Type: CNAME
Name: ui
Value: luxfi.github.io
```

### For Netlify:
```
Type: CNAME
Name: ui
Value: [your-site].netlify.app
```

## ✅ Verification

After deployment, verify the site is accessible:

1. **Check deployment status:**
   - Vercel: Dashboard shows deployment status
   - GitHub Pages: Settings → Pages shows status
   - Netlify: Dashboard shows deployment status

2. **Test the URL:**
   - Visit https://ui.lux.finance
   - Check HTTPS certificate
   - Test navigation and functionality

## 🔄 Continuous Deployment

### GitHub Actions (Automatic)
Every push to `main` branch automatically deploys via GitHub Actions.

### Manual Deploy Script
```bash
./deploy.sh
```

## 📝 Environment Variables

No environment variables required for static site deployment.

## 🐛 Troubleshooting

### DNS not resolving:
- Wait 24-48 hours for DNS propagation
- Check DNS records with: `dig ui.lux.finance`

### Build failures:
- Check Node version (20.x recommended)
- Clear node_modules: `rm -rf node_modules && npm install`
- Check build logs for errors

### 404 errors on routes:
- Ensure SPA rewrites are configured (handled in vercel.json)
- For GitHub Pages, may need to use HashRouter

## 📊 Current Status

- ✅ Site built successfully
- ✅ Vercel configuration ready
- ✅ GitHub Pages workflow configured
- ✅ CNAME file in place
- ⏳ Awaiting DNS configuration for ui.lux.finance

## 🔗 Related Links

- Production: https://ui.lux.finance (pending DNS)
- Repository: https://github.com/luxfi/consensus
- Main Site: https://lux.network