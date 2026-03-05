import React, { useState, useRef } from 'react'
import axios from 'axios'

const CHUNK_SIZE = parseFloat(import.meta.env.VITE_CHUNK_SIZE_MB || '5.5') * 1024 * 1024 // Default 5.5MB
const rawMaxFileSizeMB = Number.parseInt(import.meta.env.VITE_MAX_FILE_SIZE_MB ?? '1024', 10)
const MAX_FILE_SIZE_MB = Number.isFinite(rawMaxFileSizeMB) && rawMaxFileSizeMB > 0 ? rawMaxFileSizeMB : 1024
const MAX_FILE_SIZE_GB = MAX_FILE_SIZE_MB / 1024
const MAX_BYTES = MAX_FILE_SIZE_MB * 1024 * 1024 // Default 1GB
const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080'
const CONCURRENCY = 4
const MAX_RETRIES = 3

function formatBytes(b) {
  if (b === 0) return '0 B'
  const k = 1024
  const sizes = ['B','KB','MB','GB','TB']
  const i = Math.floor(Math.log(b) / Math.log(k))
  return parseFloat((b / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

export default function App(){
  const [status, setStatus] = useState('Idle')
  const [statusType, setStatusType] = useState('info')
  const [progress, setProgress] = useState(0)
  const [fileURL, setFileURL] = useState('')
  const [selectedFile, setSelectedFile] = useState(null)
  const [isUploading, setUploading] = useState(false)
  const [isDragging, setIsDragging] = useState(false)

  const uploadState = useRef({
    uploadId: null,
    key: null,
    totalChunks: 0,
    completedParts: [],
    queue: [],
    controllers: {},
  })

  const startWorkers = () => {
    for (let i=0;i<CONCURRENCY;i++) uploadNext()
  }

  const uploadNext = async () => {
    const s = uploadState.current
    const part = s.queue.shift()
    if (!part) return
    await uploadPart(part)
    if (s.queue.length > 0) {
      uploadNext()
    }
  }

  const uploadPart = async (partNumber) => {
    const s = uploadState.current
    const start = (partNumber-1) * CHUNK_SIZE
    const end = Math.min(start + CHUNK_SIZE, s.fileSize)
    const chunk = s.file.slice(start, end)

    let attempt = 0
    while (attempt <= MAX_RETRIES) {
      try {
        // get presigned url
        const presign = await axios.post(`${API_BASE}/presign-part`, { uploadId: s.uploadId, key: s.key, partNumber })
        const url = presign.data.url

        const controller = new AbortController()
        s.controllers[partNumber] = controller

        const res = await fetch(url, { method: 'PUT', body: chunk, signal: controller.signal, headers: { 'Content-Type': s.file.type || 'application/octet-stream' } })
        if (!res.ok) throw new Error('Upload failed status ' + res.status)
        const etag = res.headers.get('etag') || ''
        s.completedParts.push({ ETag: etag, PartNumber: partNumber })
        delete s.controllers[partNumber]

        const done = s.completedParts.length
        setProgress(Math.round((done / s.totalChunks) * 100))
        if (done === s.totalChunks) await finalize()
        return
      } catch (err) {
        attempt++
        if (attempt > MAX_RETRIES) {
          setStatus('Failed uploading part ' + partNumber)
          setStatusType('error')
          setUploading(false)
          return
        }
        // backoff
        await new Promise(r => setTimeout(r, 500 * attempt))
      }
    }
  }

  const finalize = async () => {
    const s = uploadState.current
    try {
      // sort parts
      s.completedParts.sort((a,b) => a.PartNumber - b.PartNumber)
      const response = await axios.post(`${API_BASE}/complete-multipart`, { key: s.key, uploadId: s.uploadId, parts: s.completedParts })
      setStatus('Upload completed successfully!')
      setStatusType('success')
      setUploading(false)
      // Use downloadUrl from response if available, otherwise fall back to public URL
      setFileURL(response.data.downloadUrl || s.fileURL || '')
    } catch (err) {
      console.error('Error finalizing upload:', err)
      const errorMsg = err.response?.data || err.message || 'Unknown error finalizing upload'
      setStatus(`Failed to complete upload: ${errorMsg}`)
      setStatusType('error')
      setUploading(false)
    }
  }

  const handleFileSelect = (file) => {
    if (!file) return
    const maxFileSizeGB = parseInt(import.meta.env.VITE_MAX_FILE_SIZE_MB || '1024') / 1024
    if (file.size > MAX_BYTES) { 
      setStatus(`File too large (max ${maxFileSizeGB}GB)`) 
      setStatusType('error')
      return 
    }
    setSelectedFile(file)
    setStatus('')
    setStatusType('info')
    setFileURL('')
    setProgress(0)
    initiateUpload(file)
  }

  const initiateUpload = async (file) => {
    setStatus('Initializing upload...')
    setStatusType('info')
    try {
      const startRes = await axios.post(`${API_BASE}/initiate-multipart`, { key: file.name, size: file.size })
      const { uploadId, key, url } = startRes.data
      uploadState.current = { ...uploadState.current, uploadId, key, file, fileSize: file.size, fileURL: url, totalChunks: Math.ceil(file.size / CHUNK_SIZE), completedParts: [], queue: [] , controllers: {} }
      const total = uploadState.current.totalChunks
      uploadState.current.queue = Array.from({length: total}, (_,i) => i+1)
      setUploading(true)
      setStatus(`Uploading ${total} parts...`)
      setStatusType('info')
      setProgress(0)
      startWorkers()
    } catch (err) {
      console.error('Error initiating upload:', err)
      const errorMsg = err.response?.data || err.message || 'Failed to initiate upload'
      setStatus(errorMsg)
      setStatusType('error')
      setSelectedFile(null)
    }
  }

  const handleFile = async (e) => {
    const file = e.target.files[0]
    handleFileSelect(file)
  }

  const handleDragOver = (e) => {
    e.preventDefault()
    setIsDragging(true)
  }

  const handleDragLeave = (e) => {
    e.preventDefault()
    setIsDragging(false)
  }

  const handleDrop = (e) => {
    e.preventDefault()
    setIsDragging(false)
    const file = e.dataTransfer.files[0]
    handleFileSelect(file)
  }



  const handleRemoveFile = () => {
    setSelectedFile(null)
    setStatus('Idle')
    setStatusType('info')
    setProgress(0)
    setFileURL('')
    setUploading(false)
    uploadState.current = {
      uploadId: null,
      key: null,
      totalChunks: 0,
      completedParts: [],
      queue: [],
      controllers: {},
    }
  }

  const copyToClipboard = () => {
    navigator.clipboard.writeText(fileURL)
    setStatus('Link copied to clipboard!')
    setStatusType('success')
  }

  return (
    <div className="app">
      <div className="header">
        <div className="header-icon">
          <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
          </svg>
        </div>
        <h1>Direct S3 Chunked Uploader</h1>
        <p>Upload files up to {parseInt(import.meta.env.VITE_MAX_FILE_SIZE_MB || '1024') / 1024}GB with chunked uploads</p>
      </div>

      <div className="card">
        {!selectedFile && (
          <div 
            className={`upload-zone ${isDragging ? 'dragging' : ''}`}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => document.getElementById('file-input').click()}
          >
            <div className="upload-zone-icon">
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
              </svg>
            </div>
            <h3>Drop your file here or click to browse</h3>
            <p>Supports files up to {parseInt(import.meta.env.VITE_MAX_FILE_SIZE_MB || '1024') / 1024}GB</p>
            <input 
              type="file" 
              id="file-input"
              onChange={handleFile} 
            />
          </div>
        )}

        {selectedFile && (
          <div className="file-info">
            <div className="file-icon">
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
              </svg>
            </div>
            <div className="file-details">
              <div className="file-name">{selectedFile.name}</div>
              <div className="file-size">{formatBytes(selectedFile.size)}</div>
            </div>
            {!isUploading && (
              <button className="file-remove" onClick={handleRemoveFile}>
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" width="20" height="20">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>
        )}

        {(isUploading || progress > 0) && (
          <div className="progress-section visible">
            <div className="progress-header">
              <span className="progress-label">Upload Progress</span>
              <span className="progress-percentage">{progress}%</span>
            </div>
            <div className="progress-track">
              <div className={`progress-bar ${progress === 100 ? 'complete' : ''}`} style={{width: progress + '%'}}></div>
            </div>
            <div className="progress-stats">
              <span>
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" width="16" height="16">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z" />
                </svg>
                {uploadState.current.completedParts?.length || 0} / {uploadState.current.totalChunks} parts
              </span>
              <span>Uploading...</span>
            </div>
          </div>
        )}

        {status && (
          <div className={`status status-${statusType}`}>
            {statusType === 'info' && (
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
              </svg>
            )}
            {statusType === 'success' && (
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            )}
            {statusType === 'warning' && (
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
              </svg>
            )}
            {statusType === 'error' && (
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
              </svg>
            )}
            {status}
          </div>
        )}

        <div className="controls"></div>

        {fileURL && (
          <div className="result">
            <div className="result-header">
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              Upload Complete!
            </div>
            <div className="result-url">
              <a href={fileURL} target="_blank" rel="noreferrer">{fileURL}</a>
              <button className="copy-btn" onClick={copyToClipboard}>
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" width="18" height="18">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 17.25v3.375c0 .621-.504 1.125-1.125 1.125h-9.75a1.125 1.125 0 01-1.125-1.125V7.875c0-.621.504-1.125 1.125-1.125H6.75a9.06 9.06 0 011.5.124m7.5 10.376h3.375c.621 0 1.125-.504 1.125-1.125V11.25c0-4.46-3.243-8.161-7.5-8.876a9.06 9.06 0 00-1.5-.124H9.375c-.621 0-1.125.504-1.125 1.125v3.5m7.5 10.375H9.375a1.125 1.125 0 01-1.125-1.125v-9.25m12 6.625v-1.875a3.375 3.375 0 00-3.375-3.375h-1.5a1.125 1.125 0 01-1.125-1.125v-1.5a3.375 3.375 0 00-3.375-3.375H9.75" />
                </svg>
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
