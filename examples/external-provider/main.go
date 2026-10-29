package main
import(
 "context"
 "os"
 sdk "github.com/GGingGGang/dashboard/sdk"
 "github.com/GGingGGang/dashboard/sdk/protocol"
)
type inventory struct{}
func(*inventory)Check(context.Context)error{return nil}
func(*inventory)ReadData(_ context.Context,input map[string]any)(sdk.DataResult,error){return sdk.DataResult{Type:"table",Columns:[]sdk.DataColumn{{Name:"region",Type:"string"},{Name:"items",Type:"number"}},Rows:[][]any{{input["region"],42}}},nil}
func main(){_ = protocol.Serve(os.Stdin,os.Stdout,sdk.Definition{Info:sdk.ProviderInfo{Kind:"example-inventory",Name:"Inventory example",Capabilities:[]string{"data"},OnDemand:true,DefaultAuth:"none",AuthMethods:[]string{"none"}},Create:func(sdk.Connection,string)(sdk.Provider,error){return &inventory{},nil}})}
