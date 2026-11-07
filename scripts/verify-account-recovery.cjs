#!/usr/bin/env node
'use strict'
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')

const source = fs.readFileSync('web/dist/v3.js', 'utf8')

function makePage(handler) {
  const calls = []
  const toasts = []
  const context = vm.createContext({
    state: {accounts: [{id: 7, name: 'test'}], page: 'accounts'},
    document: {addEventListener() {}},
    setTimeout,
    clearTimeout,
    confirm: () => true,
    toast: message => toasts.push(message),
    // In this isolated VM test the DOM dialog is not loaded. Exercise
    // the identical mutation/verification sequence without a browser.
    XportOperation: {
      ask: async () => true,
      run: async ({perform, verify}) => {
        const result = await perform()
        if (verify) await verify(result)
        return result
      }
    },
    api: async (path, options) => {
      calls.push([path, options?.method || 'GET'])
      return handler(path, options)
    }
  })
  vm.runInContext(source, context, {filename: 'v3.js'})
  vm.runInContext('renderAccountsV3=()=>{};loadOverview=async()=>{};loadOnlineConnectionsV3=async()=>{}', context)
  return {context, calls, toasts}
}

function responseError(status, message = 'upstream failure') {
  const err = new Error(message)
  err.status = status
  return err
}

async function testRecoveredDelete() {
  const page = makePage(async (path, options) => {
    if(options?.method === 'DELETE')throw responseError(502, 'connect refused')
    if(path === '/api/health')return {ok:true}
    if(path === '/api/accounts')return {accounts:[]}
    if(path === '/api/overview')return {system:{xrayActive:true}}
    throw new Error('Unexpected API call: '+path)
  })
  await vm.runInContext('deleteAccountV3(7)', page.context)
  assert.equal(page.calls.filter(([path, method]) => path.endsWith('/7') && method === 'DELETE').length, 1)
  assert.equal(page.context.state.accounts.length, 0)
  assert(page.toasts.includes('账号已删除'))
}

async function testFailedDeleteNotRetried() {
  const page = makePage(async (path, options) => {
    if(options?.method === 'DELETE')throw responseError(502)
    if(path === '/api/health')return {ok:true}
    if(path === '/api/accounts')return {accounts:[{id:7,name:'test'}]}
    throw new Error('Unexpected API call: '+path)
  })
  await vm.runInContext('deleteAccountV3(7)', page.context)
  assert.equal(page.calls.filter(([path, method]) => path.endsWith('/7') && method === 'DELETE').length, 1)
  assert.equal(page.context.state.accounts.length, 1)
  assert(page.toasts.some(message => message.includes('未确认')))
}

async function testXrayDownNotReportedAsSuccess() {
  const page = makePage(async (path) => {
    if(path === '/api/health')return {ok:true}
    if(path === '/api/overview')return {system:{xrayActive:false}}
    throw new Error('Unexpected API call: '+path)
  })
  page.context.failure = responseError(503)
  vm.runInContext('waitForXrayV3=async()=>false', page.context)
  await assert.rejects(
    vm.runInContext('mutationWithRecoveryV3(()=>Promise.reject(failure),()=>Promise.resolve(true))', page.context),
    /Xray 未恢复/

  )
}

async function testValidationErrorNotRetried() {
  const page = makePage(async () => {throw new Error('Unexpected retry')})
  page.context.failure = responseError(400, 'invalid port')
  await assert.rejects(
    vm.runInContext('mutationWithRecoveryV3(()=>Promise.reject(failure),()=>Promise.resolve(true))', page.context),
    /invalid port/
  )
  assert.equal(page.calls.length, 0)
}

async function testBackend502PreservesError() {
  const page = makePage(async () => {throw new Error('Network recovery not expected')})
  page.context.failure = Object.assign(responseError(502, 'Xray rollback failed'), {serverError: true})
  await assert.rejects(
    vm.runInContext('mutationWithRecoveryV3(()=>Promise.reject(failure),()=>Promise.resolve(true))', page.context),
    /Xray rollback failed/
  )
  assert.equal(page.calls.length, 0)
}

async function main() {
  await testRecoveredDelete()
  await testFailedDeleteNotRetried()
  await testXrayDownNotReportedAsSuccess()
  await testValidationErrorNotRetried()
  await testBackend502PreservesError()
  console.log('X-port account recovery regression: 5 scenarios passed')
}
main().catch(error => {console.error(error);process.exitCode=1})
