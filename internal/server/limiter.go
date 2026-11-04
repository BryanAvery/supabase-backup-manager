package server

import (
 "net"
 "net/http"
 "strconv"
 "sync"
 "time"
)

type requestWindow struct{
 count int
 until time.Time
}

// limitRequests is bounded defence-in-depth for the local development API.
// This is not a replacement for network perimeter controls or IdP throttling.
func limitRequests(next http.Handler,maxRequests int,period time.Duration)http.Handler{
 if maxRequests<1 || period<=0 {panic("invalid request limiter configuration")}
 var mu sync.Mutex
 windows:=make(map[string]requestWindow)
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path=="/api/v1/health"{next.ServeHTTP(w,r);return}
  host,_,err:=net.SplitHostPort(r.RemoteAddr)
  if err!=nil || host==""{host="unknown"}
  now:=time.Now()
  mu.Lock()
  v,exists:=windows[host]
  if !exists || !now.Before(v.until){v=requestWindow{until:now.Add(period)}}
  v.count++
  if len(windows)>2048 {
   // Fail closed to prevent unbounded growth instead of evicting callers
   // that may be over limit.
   mu.Unlock()
   w.Header().Set("Retry-After","60")
   apiError(w,http.StatusTooManyRequests,"rate_limited")
   return
  }
  windows[host]=v
  denied:=v.count>maxRequests
  remaining:=time.Until(v.until)
  mu.Unlock()
  if denied{
   seconds:=int(remaining.Seconds())+1
   if seconds<1{seconds=1}
   w.Header().Set("Retry-After",strconv.Itoa(seconds))
   apiError(w,http.StatusTooManyRequests,"rate_limited")
   return
  }
  next.ServeHTTP(w,r)
 })
}
