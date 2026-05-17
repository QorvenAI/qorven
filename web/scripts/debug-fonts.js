#!/usr/bin/env node
const puppeteer = require('puppeteer');

const URL = process.argv[2] || 'http://localhost:3000/qors/67c12df5-7866-42d0-9cd3-cb4702b93076';
const MSG = process.argv[3] || 'hello';

(async () => {
  const browser = await puppeteer.launch({ headless: true, args: ['--no-sandbox'] });
  const page = await browser.newPage();
  
  console.log('Opening:', URL);
  await page.goto(URL, { waitUntil: 'networkidle2' });
  
  // Send a message
  console.log('Sending message:', MSG);
  await page.waitForSelector('textarea');
  await page.type('textarea', MSG);
  await page.keyboard.press('Enter');
  
  // Wait for streaming to start
  await page.waitForTimeout(2000);
  
  // Capture font styles DURING streaming
  console.log('\n=== DURING STREAMING ===');
  const streamingStyles = await page.evaluate(() => {
    const results = [];
    // Find prose elements
    document.querySelectorAll('[class*="prose"]').forEach((el, i) => {
      const style = window.getComputedStyle(el);
      results.push({
        index: i,
        classes: el.className,
        fontSize: style.fontSize,
        lineHeight: style.lineHeight,
        fontFamily: style.fontFamily.slice(0, 50),
        text: el.textContent?.slice(0, 100)
      });
    });
    return results;
  });
  streamingStyles.forEach(s => console.log(JSON.stringify(s, null, 2)));
  
  // Wait for completion
  console.log('\nWaiting for completion...');
  await page.waitForTimeout(10000);
  
  // Capture font styles AFTER completion
  console.log('\n=== AFTER COMPLETION ===');
  const finalStyles = await page.evaluate(() => {
    const results = [];
    document.querySelectorAll('[class*="prose"]').forEach((el, i) => {
      const style = window.getComputedStyle(el);
      results.push({
        index: i,
        classes: el.className,
        fontSize: style.fontSize,
        lineHeight: style.lineHeight,
        fontFamily: style.fontFamily.slice(0, 50),
        text: el.textContent?.slice(0, 100)
      });
    });
    return results;
  });
  finalStyles.forEach(s => console.log(JSON.stringify(s, null, 2)));
  
  // Take screenshot
  await page.screenshot({ path: '/tmp/debug-fonts.png', fullPage: true });
  console.log('\nScreenshot saved to /tmp/debug-fonts.png');
  
  await browser.close();
})();
