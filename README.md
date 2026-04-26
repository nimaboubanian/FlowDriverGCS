# FlowDriverGCS

- [English Version](#english-version)
- [نسخه فارسی](#persian-version)

---

## English Version

**FlowDriverGCS** is a covert transport system designed to tunnel network traffic (SOCKS5) through Google Cloud Storage (GCS). It leverages legitimate API traffic to establish reliable communication in restrictive network environments. 

This project is a fork of the original [FlowDriver](https://github.com/NullLatency/FlowDriver) project. While the original relies on Google Drive, this fork uses Google Cloud Storage (GCS) to resolve severe API rate limit issues, providing significantly higher throughput and stability.

### Disclaimer
This project is intended for personal usage and research purposes only. Do not use it for illegal purposes or in production environments. The authors are not responsible for any misuse.

### Prerequisites
- A Google Cloud account with billing enabled

### Step-by-Step Setup Guide

#### 1. Google Cloud Setup
1. Go to the Google Cloud Console and create a new project.
2. Navigate to **Cloud Storage** -> **Buckets** and click **Create**.
3. Choose a unique name for your bucket and select the **Standard** storage class.
4. Navigate to **APIs & Services** -> **Credentials**.
5. Click **Create Credentials** -> **OAuth client ID**. Select **Desktop App** as the type.
6. Download the resulting JSON file and rename it exactly to `credentials.json`. Place this file in the project folder.

#### 2. Download Binaries
You do not need to install Go or build the project from source. 
1. Go to the **Releases** page of this GitHub repository.
2. Download the `.zip` file that matches your operating system (Linux, Windows, or macOS).
3. Extract the contents of the `.zip` file into a new folder. You will find the pre-built `client` and `server` executables inside, along with example configuration files.

#### 3. Configuration
You need to create configuration files for both the client and the server. You can use the provided examples as a starting point.

Create `client_config.json`:
```json
{
  "listen_addr": "127.0.0.1:1080",
  "storage_type": "gcs",
  "gcs_bucket": "YOUR_BUCKET_NAME",
  "refresh_rate_ms": 100,
  "flush_rate_ms": 200,
  "transport": {
    "TargetIP": "216.239.38.120:443",
    "SNI": "google.com",
    "HostHeader": "www.googleapis.com"
  }
}
```

Create `server_config.json`:
```json
{
  "storage_type": "gcs",
  "gcs_bucket": "YOUR_BUCKET_NAME",
  "refresh_rate_ms": 100,
  "flush_rate_ms": 200
}
```
*Note: Replace `YOUR_BUCKET_NAME` with the exact name of the bucket you created in step 1.*

#### 4. Run the Application

**First-time Authentication (Client):**
Run the client first to authenticate your Google account.
```bash
./client -c client_config.json -gc credentials.json
```
*(On Windows, use `client.exe` instead of `./client`)*
Follow the URL provided in the terminal, log in to your Google account, and paste the redirected URL back into the terminal. This will generate a `.token` file.

**Deploying the Server:**
Copy both your `credentials.json` and the newly generated `.token` file to your remote server.
Run the server:
```bash
./server -c server_config.json -gc credentials.json
```
*(On Windows, use `server.exe` instead of `./server`)*

### Cost Awareness
Google Cloud Storage is a paid service. However, for typical FlowDriver usage (small temporary files that are immediately deleted), the costs are minimal (typically under $1 per month for a single user).

---

## Persian Version

**FlowDriverGCS** یک سیستم انتقال پنهان است که برای تونل کردن ترافیک شبکه (SOCKS5) از طریق Google Cloud Storage (GCS) طراحی شده است. این سیستم با استفاده از ترافیک قانونی API، ارتباطی پایدار در محیط‌های شبکه‌ای محدود فراهم می‌کند.

این پروژه فورکی از پروژه اصلی [FlowDriver](https://github.com/NullLatency/FlowDriver) است. در حالی که نسخه اصلی به گوگل درایو متکی است، این فورک از Google Cloud Storage (GCS) استفاده می‌کند تا مشکل محدودیت شدید درخواست‌های API را برطرف کرده و سرعت و پایداری بسیار بالاتری ارائه دهد.

### سلب مسئولیت
این پروژه صرفاً برای استفاده شخصی و اهداف تحقیقاتی در نظر گرفته شده است. از آن برای مقاصد غیرقانونی یا در محیط‌های عملیاتی استفاده نکنید. نویسندگان هیچ مسئولیتی در قبال سوء استفاده از این ابزار ندارند.

### پیش‌نیازها
- یک حساب Google Cloud فعال

### راهنمای گام به گام نصب و راه‌اندازی

#### ۱. تنظیمات Google Cloud
1. به کنسول Google Cloud بروید و یک پروژه جدید ایجاد کنید.
2. به بخش **Cloud Storage** -> **Buckets** بروید و روی **Create** کلیک کنید.
3. یک نام یکتا برای باکت خود انتخاب کنید و کلاس ذخیره‌سازی را روی **Standard** قرار دهید.
4. به بخش **APIs & Services** -> **Credentials** بروید.
5. روی **Create Credentials** -> **OAuth client ID** کلیک کنید. نوع برنامه را **Desktop App** انتخاب کنید.
6. فایل JSON تولید شده را دانلود کرده و نام آن را دقیقاً به `credentials.json` تغییر دهید. این فایل را در پوشه پروژه قرار دهید.

#### ۲. دانلود فایل‌های اجرایی
شما نیازی به نصب Go یا کامپایل پروژه ندارید.
۱. به بخش **Releases** در همین مخزن گیت‌هاب بروید.
۲. فایل `.zip` مربوط به سیستم‌عامل خود (لینوکس، ویندوز یا مک) را دانلود کنید.
۳. محتویات فایل `.zip` را از حالت فشرده خارج کنید. فایل‌های اجرایی `client` و `server` به همراه نمونه‌های پیکربندی در آن قرار دارند.

#### ۳. پیکربندی
شما باید فایل‌های پیکربندی را برای کلاینت و سرور ایجاد کنید. می‌توانید از مثال‌های زیر استفاده کنید.

ایجاد فایل `client_config.json`:
```json
{
  "listen_addr": "127.0.0.1:1080",
  "storage_type": "gcs",
  "gcs_bucket": "YOUR_BUCKET_NAME",
  "refresh_rate_ms": 100,
  "flush_rate_ms": 200,
  "transport": {
    "TargetIP": "216.239.38.120:443",
    "SNI": "google.com",
    "HostHeader": "www.googleapis.com"
  }
}
```

ایجاد فایل `server_config.json`:
```json
{
  "storage_type": "gcs",
  "gcs_bucket": "YOUR_BUCKET_NAME",
  "refresh_rate_ms": 100,
  "flush_rate_ms": 200
}
```
*توجه: به جای `YOUR_BUCKET_NAME` نام باکتی که در مرحله ۱ ساختید را قرار دهید.*

#### ۴. اجرای برنامه

**احراز هویت برای اولین بار (کلاینت):**
ابتدا کلاینت را برای احراز هویت حساب گوگل خود اجرا کنید.
```bash
./client -c client_config.json -gc credentials.json
```
*(در سیستم‌عامل ویندوز، به جای `./client` از `client.exe` استفاده کنید)*
لینکی که در ترمینال نمایش داده می‌شود را دنبال کنید، وارد حساب گوگل خود شوید و URL تغییر مسیر داده شده را کپی کرده و در ترمینال جای‌گذاری کنید. این کار یک فایل `.token` تولید می‌کند.

**راه‌اندازی سرور:**
فایل `credentials.json` و فایل `.token` که به تازگی تولید شده را در سرور خود کپی کنید.
سرور را اجرا کنید:
```bash
./server -c server_config.json -gc credentials.json
```
*(در سیستم‌عامل ویندوز، به جای `./server` از `server.exe` استفاده کنید)*

### آگاهی از هزینه‌ها
سرویس Google Cloud Storage رایگان نیست. با این حال، برای استفاده معمول از FlowDriver (فایل‌های موقت کوچک که فوراً حذف می‌شوند)، هزینه‌ها حداقل است (معمولاً کمتر از ۱ دلار در ماه برای یک کاربر).
