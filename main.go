package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func main() {
	cobra.EnableCommandSorting = false

	rootCmd := &cobra.Command{
		Use: filepath.Base(os.Args[0]),
	}

	serverCmd := &cobra.Command{
		Use: `server`,
		Run: func(cmd *cobra.Command, args []string) {
			listen, _ := cmd.Flags().GetString(`listen`)
			token, _ := cmd.Flags().GetString(`token`)
			s := NewServer(token)
			if err := http.ListenAndServe(listen, s); err != nil {
				log.Fatalln(err)
			}
		},
	}
	serverCmd.Flags().StringP(`listen`, `l`, `localhost:8888`, `服务器 HTTP2TCP 监听地址`)
	serverCmd.Flags().StringP(`token`, `t`, ``, `密钥`)
	rootCmd.AddCommand(serverCmd)

	clientCmd := &cobra.Command{
		Use: `client`,
		Run: func(cmd *cobra.Command, args []string) {
			listen, _ := cmd.Flags().GetString(`listen`)
			server, _ := cmd.Flags().GetString(`server`)
			token, _ := cmd.Flags().GetString(`token`)
			c := NewClient(server, token)
			c.ListenAndServe(listen)
		},
	}
	clientCmd.Flags().StringP(`listen`, `l`, `localhost:1080`, `本地 SOCKS 协议监听端口`)
	clientCmd.Flags().StringP(`server`, `s`, `http://localhost:8888`, `远程服务器端`)
	clientCmd.Flags().StringP(`token`, `t`, ``, `密钥`)
	rootCmd.AddCommand(clientCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
